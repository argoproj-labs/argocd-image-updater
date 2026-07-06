package image

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	godigest "github.com/opencontainers/go-digest"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

// sigstoreBundleType is the OCI artifactType used by cosign ≥ 2.x when
// storing signatures via the OCI Referrers API (sigstore bundle / DSSE format).
const sigstoreBundleType = "application/vnd.dev.sigstore.bundle.v0.3+json"

// ociImageIndexMediaType is the media type of an OCI Image Index (manifest list).
// cosign 2.x uses an Image Index as the container for tag-based fallback
// signature storage: the index itself is stored at "sha256-<hex>.sig" and
// each of its manifest entries is a sigstore bundle manifest.
const ociImageIndexMediaType = "application/vnd.oci.image.index.v1+json"

// simpleSigningMediaType is the layer media type of cosign's legacy
// "Simple Signing" format (SIGNATURE_SPEC.md), used for local key-based
// signing (cosign sign --key, no Fulcio/OIDC) when pushing to a registry
// that does not support the OCI Referrers API. Unlike the sigstore bundle
// format, the signature is a plain base64 annotation on the layer
// descriptor and the payload is signed directly (no DSSE envelope/PAE).
const simpleSigningMediaType = "application/vnd.dev.cosign.simplesigning.v1+json"

// simpleSigningSigAnnotation is the annotation key holding the base64-encoded
// ECDSA signature over sha256(payload) for the Simple Signing format.
const simpleSigningSigAnnotation = "dev.cosignproject.cosign/signature"

// Verify carries the signature verification policy for a single image.
type Verify struct {
	// CosignKey is the PEM-encoded ECDSA public key.
	CosignKey string
}

// RegistryFetcher is the subset of registry.RegistryClient required for
// signature verification. The concrete *registry.registryClient satisfies
// this interface automatically — no import of the registry package needed here.
type RegistryFetcher interface {
	// ManifestForTag resolves the image manifest when ManifestDigest is not
	// yet cached on the ImageTag (slow path only).
	ManifestForTag(ctx context.Context, tagStr string) (distribution.Manifest, error)

	// ManifestForDigest fetches the OCI manifest identified by dgst. Used to
	// retrieve the cosign signature artifact manifest found via Referrers.
	ManifestForDigest(ctx context.Context, dgst godigest.Digest) (distribution.Manifest, error)

	// Referrers calls GET /v2/<repo>/referrers/<digest> (OCI Distribution Spec
	// v1.1) and returns all referrer descriptors for the given manifest digest.
	Referrers(ctx context.Context, dgst godigest.Digest) ([]distribution.Descriptor, error)

	// BlobContent downloads the raw bytes of the blob identified by dgst.
	// Required for reading the sigstore bundle JSON from the OCI layer blob.
	BlobContent(ctx context.Context, dgst godigest.Digest) ([]byte, error)
}

// ---------------------------------------------------------------------------
// Sigstore bundle types (minimal structs for DSSE parsing)
// ---------------------------------------------------------------------------

// sigstoreBundle is a minimal representation of the sigstore bundle v0.3 JSON.
// We only need the DSSE envelope for key-based verification.
type sigstoreBundle struct {
	DSSEEnvelope *dsseEnvelope `json:"dsseEnvelope,omitempty"`
}

// dsseEnvelope is the Dead Simple Signing Envelope stored inside the bundle.
type dsseEnvelope struct {
	Payload     string          `json:"payload"`
	PayloadType string          `json:"payloadType"`
	Signatures  []dsseSignature `json:"signatures"`
}

// dsseSignature holds one signer's base64-encoded ECDSA signature.
type dsseSignature struct {
	Sig string `json:"sig"`
}

// simpleSigningPayload is the minimal representation of the cosign/sigstore
// simple-signing JSON embedded inside a DSSE envelope payload field.
// Used to bind the verified signature to the specific image manifest digest and
// prevent a valid bundle signed for image A from being replayed onto image B.
type simpleSigningPayload struct {
	Critical struct {
		Image struct {
			DockerManifestDigest string `json:"docker-manifest-digest"`
		} `json:"image"`
	} `json:"critical"`
}

// inTotoStatement is the minimal representation of an in-toto v1 Statement
// (https://in-toto.io/Statement/v1). Plain `cosign sign` (no --attest) using
// the modern sigstore bundle format wraps this in the DSSE envelope instead
// of the legacy simple-signing JSON — its predicateType is
// "https://sigstore.dev/cosign/sign/v1" and the subject digest is what binds
// the signature to a specific image manifest.
type inTotoStatement struct {
	Type    string `json:"_type"`
	Subject []struct {
		Digest map[string]string `json:"digest"`
	} `json:"subject"`
}

// ---------------------------------------------------------------------------
// VerifyWithPublicKey
// ---------------------------------------------------------------------------

// VerifyWithPublicKey verifies that the cosign signature on img was produced
// with the ECDSA private key corresponding to the PEM public key in verifyConfig.
//
// If img.ImageTag.TagSignatures is already populated (e.g. from a previous call),
// the network fetch is skipped and the cached candidates are used directly.
// Verification succeeds as soon as any one candidate matches the configured key.
//
// regClient must already have NewRepository called for the image's repository.
func VerifyWithPublicKey(ctx context.Context, img *ContainerImage, verifyConfig *Verify, regClient RegistryFetcher) error {
	logCtx := log.LoggerFromContext(ctx)
	imageRef := img.GetFullNameWithTag()

	if img.ImageTag == nil {
		return fmt.Errorf("image %s has no tag information", imageRef)
	}

	if len(img.ImageTag.TagSignatures) == 0 {
		logCtx.Debugf("Fetching cosign signature for %s", imageRef)
		sigs, err := fetchTagSignatures(ctx, img.ImageTag, regClient)
		if err != nil {
			return fmt.Errorf("failed to fetch cosign signature for %s: %w", imageRef, err)
		}
		img.ImageTag.TagSignatures = sigs
	} else {
		logCtx.Debugf("Using %d cached cosign signature candidate(s) for %s", len(img.ImageTag.TagSignatures), imageRef)
	}

	logCtx.Debugf("Verifying cosign signature for %s (%d candidate(s))", imageRef, len(img.ImageTag.TagSignatures))
	for _, sig := range img.ImageTag.TagSignatures {
		if err := verifySignature(imageRef, sig, verifyConfig.CosignKey); err == nil {
			logCtx.Infof("Cosign signature verified successfully for %s", imageRef)
			return nil
		}
	}
	return fmt.Errorf("signature verification failed for image %s: no matching signature found among %d candidate(s)",
		imageRef, len(img.ImageTag.TagSignatures))
}

// ---------------------------------------------------------------------------
// fetchTagSignature
// ---------------------------------------------------------------------------

// fetchTagSignature resolves the image manifest digest, queries the OCI
// Referrers API for all sigstore bundle artifacts, and collects all
// DSSE-wrapped ECDSA signatures.
//
// When the OCI Referrers API is unavailable or returns no results (e.g. the
// registry does not implement OCI Distribution Spec v1.1), the function
// automatically falls back to the tag-based storage cosign uses on such
// registries. Which tag naming convention and payload format cosign chooses
// depends on how the image was signed (keyless/bundle-based vs. local
// key-based) rather than on cosign's version alone, so both are tried:
//   - "sha256-<hex>" (no suffix): the OCI "Referrers Tag Schema" fallback for
//     bundle-based signing, used by cosign ≥ 3.x by default and optionally by
//     cosign 2.x.
//   - "sha256-<hex>.sig": the legacy cosign Signature Spec tag. This is what
//     `cosign sign --key` (no Fulcio/OIDC) still produces against a registry
//     without OCI Referrers support, on any cosign version, and it may hold
//     either a sigstore bundle/OCI Image Index or the pre-bundle legacy
//     "Simple Signing" payload — see extractSigsFromManifest.
//
// The returned slice contains one entry per signer.  Errors on individual
// referrers are logged as warnings and skipped so that a single bad artifact
// cannot block a valid signature from being found.
func fetchTagSignatures(ctx context.Context, imgTag *tag.ImageTag, regClient RegistryFetcher) ([]*tag.TagSignature, error) {
	logCtx := log.LoggerFromContext(ctx)

	// --- Step 1: resolve the image manifest digest ---
	var imgDigest godigest.Digest

	if imgTag.ManifestDigest != "" {
		d, err := godigest.Parse(imgTag.ManifestDigest)
		if err != nil {
			return nil, fmt.Errorf("invalid manifest digest %q: %w", imgTag.ManifestDigest, err)
		}
		imgDigest = d
		logCtx.Debugf("Using cached manifest digest %s for referrers lookup of tag %q", imgDigest, imgTag.TagName)
	} else {
		// Slow path: ManifestDigest not yet cached (e.g. cache miss).
		logCtx.Debugf("Manifest digest not cached for tag %q, fetching image manifest", imgTag.TagName)
		imgManifest, err := regClient.ManifestForTag(ctx, imgTag.TagName)
		if err != nil {
			return nil, fmt.Errorf("error fetching image manifest for tag %q: %w", imgTag.TagName, err)
		}
		_, imgPayload, err := imgManifest.Payload()
		if err != nil {
			return nil, fmt.Errorf("error getting image manifest payload for tag %q: %w", imgTag.TagName, err)
		}
		imgDigest = godigest.FromBytes(imgPayload)
		logCtx.Debugf("Computed manifest digest %s for tag %q", imgDigest, imgTag.TagName)
	}

	// --- Step 2: try OCI Referrers API (OCI Distribution Spec v1.1) ---
	// A non-nil error here is non-fatal: the tag-based fallback is tried below.
	logCtx.Debugf("Fetching OCI referrers for digest %s (tag %q)", imgDigest, imgTag.TagName)
	referrers, referrersErr := regClient.Referrers(ctx, imgDigest)
	if referrersErr != nil {
		logCtx.Debugf("OCI Referrers API unavailable for tag %q (digest %s): %v — will try tag-based fallback",
			imgTag.TagName, imgDigest, referrersErr)
	}

	// --- Step 3: collect signatures from ALL sigstore bundle referrers ---
	var allSigs []*tag.TagSignature
	for _, ref := range referrers {
		if ref.ArtifactType != sigstoreBundleType {
			continue
		}
		logCtx.Debugf("Found cosign signature artifact at %s for tag %q", ref.Digest, imgTag.TagName)
		sigManifest, err := regClient.ManifestForDigest(ctx, ref.Digest)
		if err != nil {
			logCtx.Warnf("error fetching cosign signature manifest %s for tag %q: %v — skipping", ref.Digest, imgTag.TagName, err)
			continue
		}
		sigs, err := extractSigsFromManifest(ctx, sigManifest, ref.Digest.String(), imgTag.TagName, imgDigest, regClient)
		if err != nil {
			logCtx.Warnf("error extracting signatures from manifest %s for tag %q: %v — skipping", ref.Digest, imgTag.TagName, err)
			continue
		}
		allSigs = append(allSigs, sigs...)
	}

	// --- Step 4: tag-based fallback for registries without OCI v1.1 Referrers ---
	// The image digest is encoded into a tag by replacing ":" with "-", either
	// with no suffix (OCI Referrers Tag Schema) or with the legacy ".sig"
	// suffix (cosign Signature Spec). Both are tried so verification works
	// regardless of how the signature was produced — see fetchTagSignatures'
	// doc comment above for details.
	if len(allSigs) == 0 {
		digestTag := strings.ReplaceAll(imgDigest.String(), ":", "-")
		for _, fallbackTag := range []string{digestTag, digestTag + ".sig"} {
			logCtx.Debugf("No OCI referrers found for tag %q; trying tag-based fallback %q", imgTag.TagName, fallbackTag)
			sigs, err := fetchSigsFromFallbackTag(ctx, fallbackTag, imgTag.TagName, imgDigest, regClient)
			if err != nil {
				logCtx.Debugf("Tag-based fallback %q not found for tag %q: %v", fallbackTag, imgTag.TagName, err)
				continue
			}
			if len(sigs) > 0 {
				logCtx.Debugf("Found %d cosign signature(s) via tag-based fallback %q for tag %q", len(sigs), fallbackTag, imgTag.TagName)
				allSigs = append(allSigs, sigs...)
				break
			}
		}
	}

	if len(allSigs) == 0 {
		if referrersErr != nil {
			// Surface the original Referrers error when both paths found nothing.
			return nil, fmt.Errorf("failed to fetch OCI referrers for digest %s: %w", imgDigest, referrersErr)
		}
		return nil, fmt.Errorf("no cosign signature found in OCI referrers or tag-based fallback for image tag %q (digest %s)", imgTag.TagName, imgDigest)
	}
	return allSigs, nil
}

// fetchSigsFromFallbackTag fetches the manifest at fallbackTag, if it exists,
// and extracts sigstore bundle signatures from it. It transparently handles
// both container formats cosign may use for the fallback artifact:
//   - an OCI Image Index whose References() entries are the actual signature
//     bundle manifests (cosign 2.x/3.x), or
//   - a plain OCI Image Manifest holding the signature bundle directly
//     (older cosign).
//
// A non-nil error means the tag does not exist or could not be read, which
// the caller treats as "try the next fallback tag convention".
func fetchSigsFromFallbackTag(ctx context.Context, fallbackTag, imgTagName string, imgDigest godigest.Digest, regClient RegistryFetcher) ([]*tag.TagSignature, error) {
	logCtx := log.LoggerFromContext(ctx)

	fallbackManifest, err := regClient.ManifestForTag(ctx, fallbackTag)
	if err != nil {
		return nil, err
	}

	fallbackMediaType, _, err := fallbackManifest.Payload()
	if err != nil {
		return nil, fmt.Errorf("error reading fallback manifest payload: %w", err)
	}

	if fallbackMediaType != ociImageIndexMediaType {
		// Plain OCI Image Manifest at the fallback tag (older cosign).
		return extractSigsFromManifest(ctx, fallbackManifest, fallbackTag, imgTagName, imgDigest, regClient)
	}

	// The fallback tag points to an OCI Image Index; each References() entry
	// is a signature bundle manifest.
	refs := fallbackManifest.References()
	logCtx.Debugf("Tag-based fallback %q is an OCI Image Index with %d entr(ies) for tag %q",
		fallbackTag, len(refs), imgTagName)

	var sigs []*tag.TagSignature
	for _, desc := range refs {
		subManifest, err := regClient.ManifestForDigest(ctx, desc.Digest)
		if err != nil {
			logCtx.Warnf("Error fetching fallback index entry %s for tag %q: %v — skipping",
				desc.Digest, imgTagName, err)
			continue
		}
		s, err := extractSigsFromManifest(ctx, subManifest, desc.Digest.String(), imgTagName, imgDigest, regClient)
		if err != nil {
			logCtx.Warnf("Error extracting signatures from fallback index entry %s for tag %q: %v — skipping",
				desc.Digest, imgTagName, err)
			continue
		}
		sigs = append(sigs, s...)
	}
	return sigs, nil
}

// extractSigsFromManifest validates the OCI manifest and delegates to the
// appropriate extractor based on the layer's media type. Returns one
// TagSignature per signer found. expectedDigest is the image manifest digest,
// forwarded for replay-attack prevention.
func extractSigsFromManifest(ctx context.Context, m distribution.Manifest, manifestRef, imgTagName string, expectedDigest godigest.Digest, regClient RegistryFetcher) ([]*tag.TagSignature, error) {
	ociSig, ok := m.(*ocischema.DeserializedManifest)
	if !ok {
		return nil, fmt.Errorf("signature manifest %q is not an OCI image manifest (got %T)", manifestRef, m)
	}
	if len(ociSig.Layers) == 0 {
		return nil, fmt.Errorf("no layers in signature manifest %q for image tag %q", manifestRef, imgTagName)
	}

	layer := ociSig.Layers[0]
	switch layer.MediaType {
	case sigstoreBundleType:
		return extractDSSEBundle(ctx, layer, imgTagName, expectedDigest, regClient)
	case simpleSigningMediaType:
		return extractSimpleSigning(ctx, layer, imgTagName, expectedDigest, regClient)
	default:
		return nil, fmt.Errorf("unsupported cosign layer media type %q in manifest %q (image tag %q)",
			layer.MediaType, manifestRef, imgTagName)
	}
}

// extractDSSEBundle fetches the sigstore bundle blob, parses the DSSE envelope,
// validates that the embedded image digest matches expectedDigest (see
// extractEmbeddedDigest for the two payload formats this supports), and
// returns one TagSignature per signer. PayloadDigest is hex(sha256(PAE)) —
// the exact bytes each private key signed.
//
// The digest binding check prevents a valid bundle signed for image A from being
// replayed as a referrer of image B and still passing ECDSA verification.
//
// DSSE Pre-Authentication Encoding (PAE):
//
//	"DSSEv1" SP LEN(payloadType) SP payloadType SP LEN(payload) SP payload
func extractDSSEBundle(ctx context.Context, layer distribution.Descriptor, imgTagName string, expectedDigest godigest.Digest, regClient RegistryFetcher) ([]*tag.TagSignature, error) {
	blobBytes, err := regClient.BlobContent(ctx, layer.Digest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch sigstore bundle blob for image tag %q: %w", imgTagName, err)
	}

	var bundle sigstoreBundle
	if err := json.Unmarshal(blobBytes, &bundle); err != nil {
		return nil, fmt.Errorf("failed to parse sigstore bundle for image tag %q: %w", imgTagName, err)
	}
	if bundle.DSSEEnvelope == nil {
		return nil, fmt.Errorf("sigstore bundle for image tag %q has no dsseEnvelope", imgTagName)
	}

	env := bundle.DSSEEnvelope
	if len(env.Signatures) == 0 {
		return nil, fmt.Errorf("sigstore bundle for image tag %q has no signatures in dsseEnvelope", imgTagName)
	}

	// Payload is base64-standard encoded in the protobuf JSON wire format.
	decodedPayload, err := base64.StdEncoding.DecodeString(env.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode dsseEnvelope payload for image tag %q: %w", imgTagName, err)
	}

	if err := checkDigestBinding(ctx, decodedPayload, imgTagName, expectedDigest, "bundle"); err != nil {
		return nil, err
	}

	// PAE digest is the same for all signers in this envelope.
	paeHash := sha256.Sum256(computePAE(env.PayloadType, decodedPayload))
	payloadDigest := hex.EncodeToString(paeHash[:])

	// Return one TagSignature per signer so the caller can try each one.
	sigs := make([]*tag.TagSignature, 0, len(env.Signatures))
	for _, s := range env.Signatures {
		sigs = append(sigs, &tag.TagSignature{
			Sig:           s.Sig,
			PayloadDigest: payloadDigest,
		})
	}
	return sigs, nil
}

// checkDigestBinding binds a signature to the exact image manifest digest
// embedded in its payload. Without this check a valid signature made for
// image A could be replayed against image B and still pass ECDSA
// verification.
//
// An embedded digest is required, not optional: this helper is also used for
// the tag-based fallback path, which has no registry-enforced subject
// binding (the fallback tag name is just a string; nothing stops a signature
// blob signed for one digest from being pushed under another digest's tag).
// A payload with no embedded digest in either supported format is therefore
// treated as a verification failure rather than silently allowed.
func checkDigestBinding(_ context.Context, payload []byte, imgTagName string, expectedDigest godigest.Digest, payloadKind string) error {
	embeddedDigest, err := extractEmbeddedDigest(payload, expectedDigest)
	if err != nil {
		return fmt.Errorf("failed to parse %s payload for image tag %q: %w", payloadKind, imgTagName, err)
	}
	if embeddedDigest == "" {
		return fmt.Errorf("%s payload for image tag %q has no embedded manifest digest (docker-manifest-digest or in-toto subject digest)",
			payloadKind, imgTagName)
	}
	if embeddedDigest != expectedDigest.String() {
		return fmt.Errorf("payload manifest digest %q does not match image manifest digest %q for image tag %q",
			embeddedDigest, expectedDigest, imgTagName)
	}
	return nil
}

// extractEmbeddedDigest returns the "<algo>:<hex>" image manifest digest
// embedded in a cosign signature payload, in whichever of the two formats
// cosign uses:
//   - legacy Simple Signing: critical.image.docker-manifest-digest
//   - in-toto Statement (used inside DSSE bundles for plain `cosign sign`):
//     subject[].digest[<algo>]
//
// Returns "" with a nil error if neither field is present, so the caller can
// decide how to treat a genuinely unbound payload.
func extractEmbeddedDigest(payload []byte, expectedDigest godigest.Digest) (string, error) {
	var stmt inTotoStatement
	if err := json.Unmarshal(payload, &stmt); err != nil {
		return "", err
	}
	if strings.HasPrefix(stmt.Type, "https://in-toto.io/Statement") {
		algo := expectedDigest.Algorithm().String()
		for _, subj := range stmt.Subject {
			if hex, ok := subj.Digest[algo]; ok && hex != "" {
				return algo + ":" + hex, nil
			}
		}
		return "", nil
	}

	var ssPayload simpleSigningPayload
	if err := json.Unmarshal(payload, &ssPayload); err != nil {
		return "", err
	}
	return ssPayload.Critical.Image.DockerManifestDigest, nil
}

// extractSimpleSigning extracts a signature stored using cosign's legacy
// "Simple Signing" format: the base64 ECDSA signature is a layer annotation,
// and it signs sha256(payload) directly — no DSSE envelope or PAE encoding
// is involved, unlike the sigstore bundle format.
func extractSimpleSigning(ctx context.Context, layer distribution.Descriptor, imgTagName string, expectedDigest godigest.Digest, regClient RegistryFetcher) ([]*tag.TagSignature, error) {
	sig, ok := layer.Annotations[simpleSigningSigAnnotation]
	if !ok || sig == "" {
		return nil, fmt.Errorf("simple-signing layer for image tag %q has no %q annotation", imgTagName, simpleSigningSigAnnotation)
	}

	payloadBytes, err := regClient.BlobContent(ctx, layer.Digest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch simple-signing payload blob for image tag %q: %w", imgTagName, err)
	}

	if err := checkDigestBinding(ctx, payloadBytes, imgTagName, expectedDigest, "simple-signing"); err != nil {
		return nil, err
	}

	payloadHash := sha256.Sum256(payloadBytes)
	return []*tag.TagSignature{{
		Sig:           sig,
		PayloadDigest: hex.EncodeToString(payloadHash[:]),
	}}, nil
}

// computePAE returns the DSSE Pre-Authentication Encoding bytes:
//
//	"DSSEv1" SP LEN(type) SP type SP LEN(body) SP body
func computePAE(payloadType string, payload []byte) []byte {
	prefix := fmt.Sprintf("DSSEv1 %d %s %d ", len(payloadType), payloadType, len(payload))
	return append([]byte(prefix), payload...)
}

// ---------------------------------------------------------------------------
// verifySignature
// ---------------------------------------------------------------------------

// verifySignature performs ECDSA verification against the pre-fetched
// TagSignature. No network calls are made here.
//
// sig.PayloadDigest must be hex(sha256(PAE(payloadType, payload))).
func verifySignature(imageRef string, sig *tag.TagSignature, pemPublicKey string) error {
	block, _ := pem.Decode([]byte(pemPublicKey))
	if block == nil {
		return fmt.Errorf("unable to PEM decode public key for image %s", imageRef)
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	ecKey, ok := pub.(*ecdsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key for image %s is not an ECDSA key", imageRef)
	}

	sigBytes, err := base64.StdEncoding.DecodeString(sig.Sig)
	if err != nil {
		return fmt.Errorf("failed to decode signature for image %s: %w", imageRef, err)
	}

	digestBytes, err := hex.DecodeString(sig.PayloadDigest)
	if err != nil {
		return fmt.Errorf("failed to decode payload digest for image %s: %w", imageRef, err)
	}

	if !ecdsa.VerifyASN1(ecKey, digestBytes, sigBytes) {
		return fmt.Errorf("signature verification failed for image %s: signature does not match public key", imageRef)
	}
	return nil
}
