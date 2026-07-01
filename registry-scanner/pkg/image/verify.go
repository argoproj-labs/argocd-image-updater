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

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	godigest "github.com/opencontainers/go-digest"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

// sigstoreBundleType is the OCI artifactType used by cosign ≥ 2.x when
// storing signatures via the OCI Referrers API (sigstore bundle / DSSE format).
const sigstoreBundleType = "application/vnd.dev.sigstore.bundle.v0.3+json"

// Verify carries the signature verification policy for a single image.
type Verify struct {
	// Method is the verification backend.
	Method string

	// PublicKeySecret is the PEM-encoded ECDSA public key.
	PublicKeySecret string
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
		if err := verifySignature(imageRef, sig, verifyConfig.PublicKeySecret); err == nil {
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
// Referrers API for all sigstore bundle artifacts, fetches every bundle
// manifest, and collects all DSSE-wrapped ECDSA signatures across all bundles.
//
// The returned slice contains one entry per signer.  Errors on individual
// referrers are logged as warnings and skipped so that a single bad artifact
// cannot block a valid signature from being found.
//
// Only the sigstore bundle format (cosign ≥ 2.x) is supported. Legacy cosign
// simple-signing (.sig tag) is not.
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

	// --- Step 2: fetch OCI referrers ---
	logCtx.Debugf("Fetching OCI referrers for digest %s (tag %q)", imgDigest, imgTag.TagName)
	referrers, err := regClient.Referrers(ctx, imgDigest)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OCI referrers for digest %s: %w", imgDigest, err)
	}

	// --- Step 3 & 4: collect signatures from ALL sigstore bundle referrers ---
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

	if len(allSigs) == 0 {
		return nil, fmt.Errorf("no cosign signature found in OCI referrers for image tag %q (digest %s)", imgTag.TagName, imgDigest)
	}
	return allSigs, nil
}

// ---------------------------------------------------------------------------
// extractSigsFromManifest / extractDSSEBundle
// ---------------------------------------------------------------------------

// extractSigsFromManifest validates the OCI manifest and delegates to the
// DSSE bundle extractor. Returns one TagSignature per signer in the envelope.
// expectedDigest is the image manifest digest and is forwarded to extractDSSEBundle
// for replay-attack prevention.
func extractSigsFromManifest(ctx context.Context, m distribution.Manifest, manifestRef, imgTagName string, expectedDigest godigest.Digest, regClient RegistryFetcher) ([]*tag.TagSignature, error) {
	ociSig, ok := m.(*ocischema.DeserializedManifest)
	if !ok {
		return nil, fmt.Errorf("signature manifest %q is not an OCI image manifest (got %T)", manifestRef, m)
	}
	if len(ociSig.Layers) == 0 {
		return nil, fmt.Errorf("no layers in signature manifest %q for image tag %q", manifestRef, imgTagName)
	}

	layer := ociSig.Layers[0]
	if layer.MediaType != sigstoreBundleType {
		return nil, fmt.Errorf("unsupported cosign layer media type %q in manifest %q (image tag %q)",
			layer.MediaType, manifestRef, imgTagName)
	}

	return extractDSSEBundle(ctx, layer, imgTagName, expectedDigest, regClient)
}

// extractDSSEBundle fetches the sigstore bundle blob, parses the DSSE envelope,
// validates that the embedded image digest matches expectedDigest, and returns
// one TagSignature per signer. PayloadDigest is hex(sha256(PAE)) — the exact
// bytes each private key signed.
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

	// Bind the signature to the exact image manifest digest embedded in the
	// simple-signing payload when the field is present.  Without this check a
	// valid bundle signed for image A could be replayed as a referrer of image B
	// and still pass ECDSA verification.
	//
	// Some cosign / sigstore-go versions or non-standard bundle formats omit the
	// field.  In that case we skip the check and rely on the OCI Referrers API's
	// own subject-digest binding (a referrer's OCI manifest must declare the
	// subject it attests to, enforced by the registry at write time).
	var ssPayload simpleSigningPayload
	if err := json.Unmarshal(decodedPayload, &ssPayload); err != nil {
		return nil, fmt.Errorf("failed to parse simple-signing payload for image tag %q: %w", imgTagName, err)
	}
	if embeddedDigest := ssPayload.Critical.Image.DockerManifestDigest; embeddedDigest != "" {
		if embeddedDigest != expectedDigest.String() {
			return nil, fmt.Errorf("payload manifest digest %q does not match image manifest digest %q for image tag %q",
				embeddedDigest, expectedDigest, imgTagName)
		}
	} else {
		log.LoggerFromContext(ctx).Debugf(
			"bundle payload for image tag %q does not contain docker-manifest-digest; relying on OCI subject binding",
			imgTagName,
		)
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
