package image

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"testing"
	"time"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	godigest "github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

type testKeyPair struct {
	priv   *ecdsa.PrivateKey
	pemPub string
}

func newTestKeyPair(t *testing.T) testKeyPair {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	return testKeyPair{
		priv:   priv,
		pemPub: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})),
	}
}

// signPAE signs sha256(PAE(payloadType, payload)) and returns a TagSignature
// ready for use with verifySignature.
func signPAE(t *testing.T, priv *ecdsa.PrivateKey, payloadType string, payload []byte) (digestHex, sigB64 string) {
	t.Helper()
	paeHash := sha256.Sum256(computePAE(payloadType, payload))
	sig, err := ecdsa.SignASN1(rand.Reader, priv, paeHash[:])
	require.NoError(t, err)
	return hex.EncodeToString(paeHash[:]), base64.StdEncoding.EncodeToString(sig)
}

// makeDSSEBundle creates a properly signed sigstore bundle for imgDigest,
// together with its OCI manifest. Returns the manifest, the blob digest (for
// mockFetcher.blobs keying), and the raw blob bytes.
// imgDigest must be the full "sha256:<hex>" digest of the image being signed;
// it is embedded in the simple-signing payload so the digest-binding check passes.
func makeDSSEBundle(t *testing.T, priv *ecdsa.PrivateKey, imgDigest string) (manifest *ocischema.DeserializedManifest, blobDigest string, blobBytes []byte) {
	t.Helper()

	payload := []byte(fmt.Sprintf(`{"critical":{"image":{"docker-manifest-digest":"%s"}}}`, imgDigest))
	payloadType := "application/vnd.dev.cosign.simplesigning.v1+json"
	paeHash := sha256.Sum256(computePAE(payloadType, payload))
	sig, err := ecdsa.SignASN1(rand.Reader, priv, paeHash[:])
	require.NoError(t, err)

	bundle := sigstoreBundle{
		DSSEEnvelope: &dsseEnvelope{
			Payload:     base64.StdEncoding.EncodeToString(payload),
			PayloadType: payloadType,
			Signatures:  []dsseSignature{{Sig: base64.StdEncoding.EncodeToString(sig)}},
		},
	}
	blobBytes, err = json.Marshal(bundle)
	require.NoError(t, err)

	dgst := godigest.FromBytes(blobBytes)
	blobDigest = dgst.String()
	manifest = &ocischema.DeserializedManifest{
		Manifest: ocischema.Manifest{
			Layers: []distribution.Descriptor{{
				MediaType: sigstoreBundleType,
				Digest:    dgst,
				Size:      int64(len(blobBytes)),
			}},
		},
	}
	return manifest, blobDigest, blobBytes
}

// makeDSSEBundleWithSigners is like makeDSSEBundle but embeds one DSSE
// signature per private key in the same envelope.
func makeDSSEBundleWithSigners(t *testing.T, privKeys []*ecdsa.PrivateKey, imgDigest string) (manifest *ocischema.DeserializedManifest, blobDigest string, blobBytes []byte) {
	t.Helper()

	payload := []byte(fmt.Sprintf(`{"critical":{"image":{"docker-manifest-digest":"%s"}}}`, imgDigest))
	payloadType := "application/vnd.dev.cosign.simplesigning.v1+json"
	paeHash := sha256.Sum256(computePAE(payloadType, payload))

	var envSigs []dsseSignature
	for _, priv := range privKeys {
		sig, err := ecdsa.SignASN1(rand.Reader, priv, paeHash[:])
		require.NoError(t, err)
		envSigs = append(envSigs, dsseSignature{Sig: base64.StdEncoding.EncodeToString(sig)})
	}

	bundle := sigstoreBundle{
		DSSEEnvelope: &dsseEnvelope{
			Payload:     base64.StdEncoding.EncodeToString(payload),
			PayloadType: payloadType,
			Signatures:  envSigs,
		},
	}
	blobBytes, err := json.Marshal(bundle)
	require.NoError(t, err)

	dgst := godigest.FromBytes(blobBytes)
	blobDigest = dgst.String()
	manifest = &ocischema.DeserializedManifest{
		Manifest: ocischema.Manifest{
			Layers: []distribution.Descriptor{{
				MediaType: sigstoreBundleType,
				Digest:    dgst,
				Size:      int64(len(blobBytes)),
			}},
		},
	}
	return manifest, blobDigest, blobBytes
}

// makeSimpleSigning creates a legacy cosign "Simple Signing" manifest (a
// single layer with mediaType simpleSigningMediaType, whose signature is a
// layer annotation rather than a DSSE envelope). Returns the manifest, the
// payload blob digest (for mockFetcher.blobs keying), and the raw blob bytes.
func makeSimpleSigning(t *testing.T, priv *ecdsa.PrivateKey, imgDigest string) (manifest *ocischema.DeserializedManifest, blobDigest string, blobBytes []byte) {
	t.Helper()

	payload := []byte(fmt.Sprintf(`{"critical":{"image":{"docker-manifest-digest":"%s"},"type":"cosign container image signature"}}`, imgDigest))
	payloadHash := sha256.Sum256(payload)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, payloadHash[:])
	require.NoError(t, err)

	dgst := godigest.FromBytes(payload)
	blobDigest = dgst.String()
	manifest = &ocischema.DeserializedManifest{
		Manifest: ocischema.Manifest{
			Layers: []distribution.Descriptor{{
				MediaType: simpleSigningMediaType,
				Digest:    dgst,
				Size:      int64(len(payload)),
				Annotations: map[string]string{
					simpleSigningSigAnnotation: base64.StdEncoding.EncodeToString(sig),
				},
			}},
		},
	}
	return manifest, blobDigest, payload
}

// bundleReferrer builds a referrer descriptor for a sigstore bundle artifact.
func bundleReferrer(sigArtifactDigest string) distribution.Descriptor {
	return distribution.Descriptor{
		MediaType:    "application/vnd.oci.image.manifest.v1+json",
		ArtifactType: sigstoreBundleType,
		Digest:       godigest.Digest(sigArtifactDigest),
	}
}

// newTestImageTag builds a ContainerImage with the given tag name and optional
// manifest digest.
func newTestImageTag(tagName, manifestDigest string) *ContainerImage {
	return &ContainerImage{
		RegistryURL: "quay.io",
		ImageName:   "org/app",
		ImageTag: &tag.ImageTag{
			TagName:        tagName,
			TagDate:        func() *time.Time { t := time.Now(); return &t }(),
			ManifestDigest: manifestDigest,
		},
	}
}

// ---------------------------------------------------------------------------
// mockFetcher implements RegistryFetcher using in-memory maps.
//
//   - manifests: keyed by tag name (ManifestForTag) or digest string (ManifestForDigest).
//   - errors: keyed by tag name or digest string.
//   - referrers: keyed by digest string.
//   - referrerErrors: keyed by digest string.
//   - blobs: keyed by digest string.
//   - blobErrors: keyed by digest string.
// ---------------------------------------------------------------------------

type mockFetcher struct {
	manifests      map[string]distribution.Manifest
	errors         map[string]error
	referrers      map[string][]distribution.Descriptor
	referrerErrors map[string]error
	blobs          map[string][]byte
	blobErrors     map[string]error
}

// mockManifest is a minimal distribution.Manifest for the slow-path test.
type mockManifest struct{ payload []byte }

func (m *mockManifest) Payload() (string, []byte, error) {
	return "application/vnd.oci.image.manifest.v1+json", m.payload, nil
}
func (m *mockManifest) References() []distribution.Descriptor { return nil }

// mockIndex simulates an OCI Image Index returned by a registry for the
// tag-based cosign 2.x fallback ("sha256-<hex>" tag → image index → bundle).
type mockIndex struct {
	refs []distribution.Descriptor
}

func (m *mockIndex) Payload() (string, []byte, error) {
	return "application/vnd.oci.image.index.v1+json", []byte(`{}`), nil
}
func (m *mockIndex) References() []distribution.Descriptor { return m.refs }

func (m *mockFetcher) ManifestForTag(_ context.Context, tagStr string) (distribution.Manifest, error) {
	if err, ok := m.errors[tagStr]; ok {
		return nil, err
	}
	if manifest, ok := m.manifests[tagStr]; ok {
		return manifest, nil
	}
	return nil, fmt.Errorf("manifest not found for tag %q", tagStr)
}

func (m *mockFetcher) ManifestForDigest(_ context.Context, dgst godigest.Digest) (distribution.Manifest, error) {
	key := dgst.String()
	if err, ok := m.errors[key]; ok {
		return nil, err
	}
	if manifest, ok := m.manifests[key]; ok {
		return manifest, nil
	}
	return nil, fmt.Errorf("manifest not found for digest %s", dgst)
}

func (m *mockFetcher) Referrers(_ context.Context, dgst godigest.Digest) ([]distribution.Descriptor, error) {
	key := dgst.String()
	if err, ok := m.referrerErrors[key]; ok {
		return nil, err
	}
	if refs, ok := m.referrers[key]; ok {
		return refs, nil
	}
	return nil, nil
}

func (m *mockFetcher) BlobContent(_ context.Context, dgst godigest.Digest) ([]byte, error) {
	key := dgst.String()
	if err, ok := m.blobErrors[key]; ok {
		return nil, err
	}
	if b, ok := m.blobs[key]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("blob not found for digest %s", dgst)
}

// ---------------------------------------------------------------------------
// verifySignature
// ---------------------------------------------------------------------------

func Test_verifySignature(t *testing.T) {
	kp := newTestKeyPair(t)
	payload := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:b793"}}}`)
	payloadType := "application/vnd.dev.cosign.simplesigning.v1+json"
	digestHex, sigB64 := signPAE(t, kp.priv, payloadType, payload)

	t.Run("valid key and signature succeeds", func(t *testing.T) {
		err := verifySignature("quay.io/org/app:1.0.0", &tag.TagSignature{
			Sig: sigB64, PayloadDigest: digestHex,
		}, kp.pemPub)
		assert.NoError(t, err)
	})

	t.Run("invalid PEM returns error", func(t *testing.T) {
		err := verifySignature("quay.io/org/app:1.0.0", &tag.TagSignature{
			Sig: sigB64, PayloadDigest: digestHex,
		}, "not-a-pem-block")
		assert.ErrorContains(t, err, "PEM decode")
	})

	t.Run("non-ECDSA key returns error", func(t *testing.T) {
		rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		pubDER, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
		require.NoError(t, err)
		rsaPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
		err = verifySignature("quay.io/org/app:1.0.0", &tag.TagSignature{
			Sig: sigB64, PayloadDigest: digestHex,
		}, rsaPEM)
		assert.ErrorContains(t, err, "not an ECDSA key")
	})

	t.Run("corrupted base64 signature returns error", func(t *testing.T) {
		err := verifySignature("quay.io/org/app:1.0.0", &tag.TagSignature{
			Sig: "!!!not-base64!!!", PayloadDigest: digestHex,
		}, kp.pemPub)
		assert.ErrorContains(t, err, "failed to decode signature")
	})

	t.Run("corrupted hex digest returns error", func(t *testing.T) {
		err := verifySignature("quay.io/org/app:1.0.0", &tag.TagSignature{
			Sig: sigB64, PayloadDigest: "gg-not-hex",
		}, kp.pemPub)
		assert.ErrorContains(t, err, "failed to decode payload digest")
	})

	t.Run("signature from a different key fails verification", func(t *testing.T) {
		otherKP := newTestKeyPair(t)
		err := verifySignature("quay.io/org/app:1.0.0", &tag.TagSignature{
			Sig: sigB64, PayloadDigest: digestHex,
		}, otherKP.pemPub)
		assert.ErrorContains(t, err, "signature verification failed")
	})
}

// ---------------------------------------------------------------------------
// fetchTagSignatures
// ---------------------------------------------------------------------------

func Test_fetchTagSignatures(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)

	const (
		imgManifestDigest = "sha256:aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234"
		sigArtifactDigest = "sha256:ccdd5678ccdd5678ccdd5678ccdd5678ccdd5678ccdd5678ccdd5678ccdd5678"
	)

	t.Run("fast path uses ManifestDigest and succeeds", func(t *testing.T) {
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}

		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := fetchTagSignatures(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	t.Run("slow path fetches image manifest when ManifestDigest is empty", func(t *testing.T) {
		imgPayload := []byte(`{"schemaVersion":2}`)
		imgDigest := godigest.FromBytes(imgPayload).String()
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgDigest)

		imgTag := &tag.ImageTag{TagName: "1.0.21"} // no ManifestDigest
		fetcher := &mockFetcher{
			manifests: map[string]distribution.Manifest{
				"1.0.21":          &mockManifest{payload: imgPayload},
				sigArtifactDigest: bundleManifest,
			},
			referrers: map[string][]distribution.Descriptor{
				imgDigest: {bundleReferrer(sigArtifactDigest)},
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := fetchTagSignatures(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	t.Run("invalid ManifestDigest format returns error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: "not-a-digest"}
		_, err := fetchTagSignatures(ctx, imgTag, &mockFetcher{})
		assert.ErrorContains(t, err, "invalid manifest digest")
	})

	t.Run("Referrers error with no tag-based fallback returns OCI referrers error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrerErrors: map[string]error{
				imgManifestDigest: fmt.Errorf("registry unavailable"),
			},
			// No fallback tag manifest — both paths fail.
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "failed to fetch OCI referrers")
	})

	t.Run("no cosign artifact in referrers returns error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {
					{ArtifactType: "application/vnd.other", Digest: godigest.Digest(sigArtifactDigest)},
				},
			},
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("empty referrers returns error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		_, err := fetchTagSignatures(ctx, imgTag, &mockFetcher{})
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("ManifestForDigest error skips referrer and returns no-signature error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			errors: map[string]error{
				sigArtifactDigest: fmt.Errorf("network timeout"),
			},
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("sig manifest is not an OCI manifest skips referrer and returns no-signature error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			// mockManifest is not *ocischema.DeserializedManifest.
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: &mockManifest{},
			},
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("sig manifest has no layers skips referrer and returns no-signature error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: &ocischema.DeserializedManifest{},
			},
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("layer has wrong media type skips referrer and returns no-signature error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		wrongManifest := &ocischema.DeserializedManifest{
			Manifest: ocischema.Manifest{
				Layers: []distribution.Descriptor{{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    godigest.Digest(sigArtifactDigest),
				}},
			},
		}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: wrongManifest,
			},
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("blob fetch error skips referrer and returns no-signature error", func(t *testing.T) {
		bundleManifest, blobDigest, _ := makeDSSEBundle(t, kp.priv, imgManifestDigest)
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: bundleManifest,
			},
			blobErrors: map[string]error{
				blobDigest: fmt.Errorf("network error"),
			},
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("second referrer matches when first referrer manifest fails", func(t *testing.T) {
		const sigArtifactDigest2 = "sha256:9911aa009911aa009911aa009911aa009911aa009911aa009911aa009911aa00"
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {
					bundleReferrer(sigArtifactDigest),  // first referrer — manifest will fail
					bundleReferrer(sigArtifactDigest2), // second referrer — has valid sig
				},
			},
			errors: map[string]error{
				sigArtifactDigest: fmt.Errorf("network timeout"),
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest2: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}
		got, err := fetchTagSignatures(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	// -----------------------------------------------------------------------
	// tag-based fallback (sha256-<hex> tag)
	// -----------------------------------------------------------------------

	// fallbackTag is the legacy cosign ≤2.5.x tag for imgManifestDigest
	// ("sha256:<hex>" → "sha256-<hex>.sig").
	const fallbackTag = "sha256-aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234.sig"
	// noSuffixFallbackTag is the cosign ≥3.x OCI Referrers Tag Schema fallback
	// tag for imgManifestDigest ("sha256:<hex>" → "sha256-<hex>", no suffix).
	const noSuffixFallbackTag = "sha256-aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234"

	t.Run("tag-based fallback succeeds via no-suffix tag (cosign 3.x)", func(t *testing.T) {
		// cosign >= 3.x stores the fallback artifact under the digest tag
		// without a ".sig" suffix (OCI Referrers Tag Schema).
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}

		fetcher := &mockFetcher{
			manifests: map[string]distribution.Manifest{
				noSuffixFallbackTag: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := fetchTagSignatures(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	t.Run("tag-based fallback succeeds via legacy Simple Signing format", func(t *testing.T) {
		// cosign key-based signing (no Fulcio/OIDC) to a registry without OCI
		// Referrers falls back to the legacy Simple Signing manifest format
		// (annotation-based signature, no DSSE envelope).
		simpleManifest, blobDigest, blobBytes := makeSimpleSigning(t, kp.priv, imgManifestDigest)
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}

		fetcher := &mockFetcher{
			manifests: map[string]distribution.Manifest{
				fallbackTag: simpleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := fetchTagSignatures(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	t.Run("tag-based fallback succeeds when OCI referrers is empty", func(t *testing.T) {
		// Registry returns no referrers (e.g. Docker Distribution v2 without OCI v1.1).
		// cosign stored the signature as a tag instead.
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}

		fetcher := &mockFetcher{
			// Referrers returns empty (nil, nil) — default for mockFetcher.
			manifests: map[string]distribution.Manifest{
				fallbackTag: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := fetchTagSignatures(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	t.Run("tag-based fallback succeeds when OCI referrers API returns error", func(t *testing.T) {
		// Registry returns a 404 / unsupported error for the Referrers endpoint.
		// The error is non-fatal; verification succeeds via the fallback tag.
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}

		fetcher := &mockFetcher{
			referrerErrors: map[string]error{
				imgManifestDigest: fmt.Errorf("404 page not found"),
			},
			manifests: map[string]distribution.Manifest{
				fallbackTag: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := fetchTagSignatures(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	t.Run("tag-based fallback with OCI Image Index (cosign 2.x) succeeds", func(t *testing.T) {
		// cosign 2.x stores the fallback tag as an OCI Image Index whose single
		// entry is the actual signature bundle manifest.
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}

		fetcher := &mockFetcher{
			manifests: map[string]distribution.Manifest{
				fallbackTag: &mockIndex{
					refs: []distribution.Descriptor{
						{Digest: godigest.Digest(sigArtifactDigest)},
					},
				},
				sigArtifactDigest: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := fetchTagSignatures(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	t.Run("tag-based fallback OCI Image Index entry fetch error is skipped", func(t *testing.T) {
		// The index exists but fetching its only sub-manifest fails.
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}

		fetcher := &mockFetcher{
			manifests: map[string]distribution.Manifest{
				fallbackTag: &mockIndex{
					refs: []distribution.Descriptor{
						{Digest: godigest.Digest(sigArtifactDigest)},
					},
				},
			},
			errors: map[string]error{
				sigArtifactDigest: fmt.Errorf("network timeout"),
			},
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("OCI referrers empty and fallback tag not found returns no-signature error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		// Empty mock: no referrers, no fallback tag.
		_, err := fetchTagSignatures(ctx, imgTag, &mockFetcher{})
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})

	t.Run("fallback tag manifest extraction error returns no-signature error", func(t *testing.T) {
		// The fallback manifest exists but has an unsupported layer media type.
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		badManifest := &ocischema.DeserializedManifest{
			Manifest: ocischema.Manifest{
				Layers: []distribution.Descriptor{{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    godigest.Digest(sigArtifactDigest),
				}},
			},
		}
		fetcher := &mockFetcher{
			manifests: map[string]distribution.Manifest{
				fallbackTag: badManifest,
			},
		}
		_, err := fetchTagSignatures(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers or tag-based fallback")
	})
}

// ---------------------------------------------------------------------------
// extractSigFromManifest
// ---------------------------------------------------------------------------

func Test_extractSigFromManifest(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)

	const (
		manifestRef     = "sha256:aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000"
		testImageDigest = "sha256:bbbb1111bbbb1111bbbb1111bbbb1111bbbb1111bbbb1111bbbb1111bbbb1111"
	)
	expectedDigest := godigest.Digest(testImageDigest)

	t.Run("not an OCI manifest returns error", func(t *testing.T) {
		_, err := extractSigsFromManifest(ctx, &mockManifest{}, manifestRef, "1.0.21", expectedDigest, &mockFetcher{})
		assert.ErrorContains(t, err, "not an OCI image manifest")
	})

	t.Run("no layers returns error", func(t *testing.T) {
		m := &ocischema.DeserializedManifest{} // zero layers
		_, err := extractSigsFromManifest(ctx, m, manifestRef, "1.0.21", expectedDigest, &mockFetcher{})
		assert.ErrorContains(t, err, "no layers in signature manifest")
	})

	t.Run("unsupported layer media type returns error", func(t *testing.T) {
		m := &ocischema.DeserializedManifest{
			Manifest: ocischema.Manifest{
				Layers: []distribution.Descriptor{{
					MediaType: "application/vnd.oci.image.layer.v1.tar+gzip",
					Digest:    godigest.Digest(manifestRef),
				}},
			},
		}
		_, err := extractSigsFromManifest(ctx, m, manifestRef, "1.0.21", expectedDigest, &mockFetcher{})
		assert.ErrorContains(t, err, "unsupported cosign layer media type")
	})

	t.Run("valid bundle layer delegates to extractDSSEBundle and succeeds", func(t *testing.T) {
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, testImageDigest)
		// bundleManifest is an *ocischema.DeserializedManifest with sigstoreBundleType layer.
		fetcher := &mockFetcher{
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := extractSigsFromManifest(ctx, bundleManifest, manifestRef, "1.0.21", expectedDigest, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})

	t.Run("valid simple-signing layer delegates to extractSimpleSigning and succeeds", func(t *testing.T) {
		simpleManifest, blobDigest, blobBytes := makeSimpleSigning(t, kp.priv, testImageDigest)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := extractSigsFromManifest(ctx, simpleManifest, manifestRef, "1.0.21", expectedDigest, fetcher)
		require.NoError(t, err)
		require.NotEmpty(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})
}

// ---------------------------------------------------------------------------
// extractSimpleSigning
// ---------------------------------------------------------------------------

func Test_extractSimpleSigning(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)
	const testImageDigest = "sha256:29d4b64da00e01ca2ce893a1d2bfe5ca6e6421c107f52fdffe6537bf760ddc03"
	expectedDigest := godigest.Digest(testImageDigest)

	t.Run("missing signature annotation returns error", func(t *testing.T) {
		layer := distribution.Descriptor{
			MediaType:   simpleSigningMediaType,
			Digest:      godigest.FromBytes([]byte(`{}`)),
			Annotations: nil,
		}
		_, err := extractSimpleSigning(ctx, layer, "1.0.21", expectedDigest, &mockFetcher{})
		assert.ErrorContains(t, err, "no")
		assert.ErrorContains(t, err, simpleSigningSigAnnotation)
	})

	t.Run("blob fetch error returns error", func(t *testing.T) {
		dgst := godigest.FromBytes([]byte(`{}`))
		layer := distribution.Descriptor{
			MediaType:   simpleSigningMediaType,
			Digest:      dgst,
			Annotations: map[string]string{simpleSigningSigAnnotation: "fakesig"},
		}
		fetcher := &mockFetcher{
			blobErrors: map[string]error{dgst.String(): fmt.Errorf("I/O error")},
		}
		_, err := extractSimpleSigning(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "failed to fetch simple-signing payload blob")
	})

	t.Run("payload digest mismatch returns error", func(t *testing.T) {
		simpleManifest, blobDigest, blobBytes := makeSimpleSigning(t, kp.priv, "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")
		layer := simpleManifest.Layers[0]
		fetcher := &mockFetcher{
			blobs: map[string][]byte{blobDigest: blobBytes},
		}
		_, err := extractSimpleSigning(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "does not match image manifest digest")
	})

	t.Run("valid simple-signing payload returns correct TagSignature", func(t *testing.T) {
		simpleManifest, blobDigest, blobBytes := makeSimpleSigning(t, kp.priv, testImageDigest)
		layer := simpleManifest.Layers[0]
		fetcher := &mockFetcher{
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := extractSimpleSigning(ctx, layer, "1.0.21", expectedDigest, fetcher)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})
}

// ---------------------------------------------------------------------------
// extractDSSEBundle
// ---------------------------------------------------------------------------

func Test_extractDSSEBundle(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)
	const (
		testImageDigest = "sha256:29d4b64da00e01ca2ce893a1d2bfe5ca6e6421c107f52fdffe6537bf760ddc03"
		otherDigest     = "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	)
	payload := []byte(fmt.Sprintf(`{"critical":{"image":{"docker-manifest-digest":"%s"}}}`, testImageDigest))
	payloadType := "application/vnd.dev.cosign.simplesigning.v1+json"
	expectedDigest := godigest.Digest(testImageDigest)

	// makeLayer is a helper that builds a distribution.Descriptor pointing at
	// specific blob bytes.
	makeLayer := func(blobBytes []byte) distribution.Descriptor {
		dgst := godigest.FromBytes(blobBytes)
		return distribution.Descriptor{
			MediaType: sigstoreBundleType,
			Digest:    dgst,
			Size:      int64(len(blobBytes)),
		}
	}

	t.Run("blob fetch error returns error", func(t *testing.T) {
		layer := makeLayer([]byte(`{}`))
		fetcher := &mockFetcher{
			blobErrors: map[string]error{layer.Digest.String(): fmt.Errorf("I/O error")},
		}
		_, err := extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "failed to fetch sigstore bundle blob")
	})

	t.Run("invalid JSON in blob returns error", func(t *testing.T) {
		blobBytes := []byte(`not-json`)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err := extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "failed to parse sigstore bundle")
	})

	t.Run("bundle has no dsseEnvelope returns error", func(t *testing.T) {
		blobBytes, err := json.Marshal(sigstoreBundle{}) // DSSEEnvelope is nil
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "no dsseEnvelope")
	})

	t.Run("dsseEnvelope has no signatures returns error", func(t *testing.T) {
		blobBytes, err := json.Marshal(sigstoreBundle{
			DSSEEnvelope: &dsseEnvelope{
				Payload:     base64.StdEncoding.EncodeToString(payload),
				PayloadType: payloadType,
				Signatures:  nil, // empty
			},
		})
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "no signatures in dsseEnvelope")
	})

	t.Run("invalid base64 payload returns error", func(t *testing.T) {
		blobBytes, err := json.Marshal(sigstoreBundle{
			DSSEEnvelope: &dsseEnvelope{
				Payload:     "!!!not-base64!!!",
				PayloadType: payloadType,
				Signatures:  []dsseSignature{{Sig: "fakesig"}},
			},
		})
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "failed to decode dsseEnvelope payload")
	})

	t.Run("payload digest mismatch returns error", func(t *testing.T) {
		// Bundle claims a DIFFERENT image digest than expectedDigest — replay-attack scenario.
		wrongPayload := []byte(fmt.Sprintf(`{"critical":{"image":{"docker-manifest-digest":"%s"}}}`, otherDigest))
		blobBytes, err := json.Marshal(sigstoreBundle{
			DSSEEnvelope: &dsseEnvelope{
				Payload:     base64.StdEncoding.EncodeToString(wrongPayload),
				PayloadType: payloadType,
				Signatures:  []dsseSignature{{Sig: "fakesig"}},
			},
		})
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "does not match image manifest digest")
	})

	t.Run("payload without docker-manifest-digest field fails closed", func(t *testing.T) {
		// The tag-based fallback path has no registry-enforced subject binding,
		// so a missing digest field must be rejected rather than silently
		// allowed — otherwise a signature for image A could be replayed under
		// image B's fallback tag.
		noDigestPayload := []byte(`{"critical":{"image":{}}}`)
		paeHash := sha256.Sum256(computePAE(payloadType, noDigestPayload))
		sig, err := ecdsa.SignASN1(rand.Reader, kp.priv, paeHash[:])
		require.NoError(t, err)

		blobBytes, err := json.Marshal(sigstoreBundle{
			DSSEEnvelope: &dsseEnvelope{
				Payload:     base64.StdEncoding.EncodeToString(noDigestPayload),
				PayloadType: payloadType,
				Signatures:  []dsseSignature{{Sig: base64.StdEncoding.EncodeToString(sig)}},
			},
		})
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "has no embedded manifest digest")
	})

	t.Run("in-toto statement payload with matching subject digest succeeds", func(t *testing.T) {
		// Real cosign >= 2.x, when doing plain `cosign sign` (no --attest)
		// against a registry without OCI Referrers support, wraps an in-toto
		// v1 Statement in the DSSE envelope instead of the legacy
		// simple-signing JSON. Verified empirically against cosign v3.1.1.
		inTotoPayloadType := "application/vnd.in-toto+json"
		inTotoPayload := []byte(fmt.Sprintf(
			`{"_type":"https://in-toto.io/Statement/v1","subject":[{"digest":{"sha256":"%s"},"annotations":{}}],"predicateType":"https://sigstore.dev/cosign/sign/v1","predicate":{}}`,
			expectedDigest.Encoded(),
		))
		paeHash := sha256.Sum256(computePAE(inTotoPayloadType, inTotoPayload))
		sig, err := ecdsa.SignASN1(rand.Reader, kp.priv, paeHash[:])
		require.NoError(t, err)

		blobBytes, err := json.Marshal(sigstoreBundle{
			DSSEEnvelope: &dsseEnvelope{
				Payload:     base64.StdEncoding.EncodeToString(inTotoPayload),
				PayloadType: inTotoPayloadType,
				Signatures:  []dsseSignature{{Sig: base64.StdEncoding.EncodeToString(sig)}},
			},
		})
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		sigs, err := extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		require.NoError(t, err)
		require.Len(t, sigs, 1)
		wantDigestHex := hex.EncodeToString(paeHash[:])
		assert.Equal(t, wantDigestHex, sigs[0].PayloadDigest)
		assert.NoError(t, verifySignature("test-image", sigs[0], kp.pemPub))
	})

	t.Run("in-toto statement payload with mismatched subject digest fails", func(t *testing.T) {
		inTotoPayloadType := "application/vnd.in-toto+json"
		inTotoPayload := []byte(fmt.Sprintf(
			`{"_type":"https://in-toto.io/Statement/v1","subject":[{"digest":{"sha256":"%s"},"annotations":{}}],"predicateType":"https://sigstore.dev/cosign/sign/v1","predicate":{}}`,
			godigest.Digest(otherDigest).Encoded(),
		))
		paeHash := sha256.Sum256(computePAE(inTotoPayloadType, inTotoPayload))
		sig, err := ecdsa.SignASN1(rand.Reader, kp.priv, paeHash[:])
		require.NoError(t, err)

		blobBytes, err := json.Marshal(sigstoreBundle{
			DSSEEnvelope: &dsseEnvelope{
				Payload:     base64.StdEncoding.EncodeToString(inTotoPayload),
				PayloadType: inTotoPayloadType,
				Signatures:  []dsseSignature{{Sig: base64.StdEncoding.EncodeToString(sig)}},
			},
		})
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		assert.ErrorContains(t, err, "does not match image manifest digest")
	})

	t.Run("valid bundle returns correct TagSignature", func(t *testing.T) {
		paeHash := sha256.Sum256(computePAE(payloadType, payload))
		sig, err := ecdsa.SignASN1(rand.Reader, kp.priv, paeHash[:])
		require.NoError(t, err)

		blobBytes, err := json.Marshal(sigstoreBundle{
			DSSEEnvelope: &dsseEnvelope{
				Payload:     base64.StdEncoding.EncodeToString(payload),
				PayloadType: payloadType,
				Signatures:  []dsseSignature{{Sig: base64.StdEncoding.EncodeToString(sig)}},
			},
		})
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}

		got, err := extractDSSEBundle(ctx, layer, "1.0.21", expectedDigest, fetcher)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, hex.EncodeToString(paeHash[:]), got[0].PayloadDigest)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got[0], kp.pemPub))
	})
}

// ---------------------------------------------------------------------------
// VerifyWithPublicKey
// ---------------------------------------------------------------------------

func Test_VerifyWithPublicKey(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)

	const (
		imgManifestDigest = "sha256:ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234"
		sigArtifactDigest = "sha256:eeff5678eeff5678eeff5678eeff5678eeff5678eeff5678eeff5678eeff5678"
	)

	verifyConfig := &Verify{CosignKey: kp.pemPub}

	t.Run("nil ImageTag returns error", func(t *testing.T) {
		img := &ContainerImage{RegistryURL: "quay.io", ImageName: "org/app"}
		err := VerifyWithPublicKey(ctx, img, verifyConfig, &mockFetcher{})
		assert.ErrorContains(t, err, "no tag information")
	})

	t.Run("uses cached TagSignatures without any network call", func(t *testing.T) {
		img := newTestImageTag("1.0.21", imgManifestDigest)
		payloadType := "application/vnd.dev.cosign.simplesigning.v1+json"
		// Content only needs to produce a verifiable ECDSA signature; digest binding
		// is not exercised on the cache-hit path.
		cachedPayload := []byte(fmt.Sprintf(`{"critical":{"image":{"docker-manifest-digest":"%s"}}}`, imgManifestDigest))
		digestHex, sigB64 := signPAE(t, kp.priv, payloadType, cachedPayload)
		img.ImageTag.TagSignatures = []*tag.TagSignature{{Sig: sigB64, PayloadDigest: digestHex}}

		err := VerifyWithPublicKey(ctx, img, verifyConfig, &mockFetcher{})
		assert.NoError(t, err)
	})

	t.Run("DSSE bundle verifies successfully end-to-end", func(t *testing.T) {
		img := newTestImageTag("1.0.21", imgManifestDigest)
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)

		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		err := VerifyWithPublicKey(ctx, img, verifyConfig, fetcher)
		assert.NoError(t, err)
		// TagSignatures are cached on the tag after fetch.
		require.NotEmpty(t, img.ImageTag.TagSignatures)
	})

	t.Run("referrers error is propagated", func(t *testing.T) {
		img := newTestImageTag("1.0.21", imgManifestDigest)
		fetcher := &mockFetcher{
			referrerErrors: map[string]error{
				imgManifestDigest: fmt.Errorf("registry timeout"),
			},
		}
		err := VerifyWithPublicKey(ctx, img, verifyConfig, fetcher)
		assert.ErrorContains(t, err, "failed to fetch cosign signature")
	})

	t.Run("no signature in referrers is propagated as error", func(t *testing.T) {
		img := newTestImageTag("1.0.21", imgManifestDigest)
		err := VerifyWithPublicKey(ctx, img, verifyConfig, &mockFetcher{})
		assert.ErrorContains(t, err, "failed to fetch cosign signature")
	})

	t.Run("DSSE bundle signed with wrong key fails verification", func(t *testing.T) {
		img := newTestImageTag("1.0.21", imgManifestDigest)
		otherKP := newTestKeyPair(t)
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, otherKP.priv, imgManifestDigest)

		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		err := VerifyWithPublicKey(ctx, img, verifyConfig, fetcher)
		assert.ErrorContains(t, err, "signature verification failed")
	})

	t.Run("second referrer signed with correct key succeeds", func(t *testing.T) {
		// Two referrer bundles: the first is signed with the wrong key, the
		// second with the correct key.  Verification must succeed.
		const sigArtifactDigest2 = "sha256:bbcc0000bbcc0000bbcc0000bbcc0000bbcc0000bbcc0000bbcc0000bbcc0000"
		img := newTestImageTag("1.0.21", imgManifestDigest)
		wrongKP := newTestKeyPair(t)
		wrongManifest, wrongBlobDigest, wrongBlobBytes := makeDSSEBundle(t, wrongKP.priv, imgManifestDigest)
		goodManifest, goodBlobDigest, goodBlobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)

		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {
					bundleReferrer(sigArtifactDigest),  // wrong key
					bundleReferrer(sigArtifactDigest2), // correct key
				},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest:  wrongManifest,
				sigArtifactDigest2: goodManifest,
			},
			blobs: map[string][]byte{
				wrongBlobDigest: wrongBlobBytes,
				goodBlobDigest:  goodBlobBytes,
			},
		}

		err := VerifyWithPublicKey(ctx, img, verifyConfig, fetcher)
		assert.NoError(t, err)
	})

	t.Run("second DSSE signature in envelope matches correct key", func(t *testing.T) {
		// Single referrer bundle whose DSSE envelope contains two signatures:
		// the first from a wrong key, the second from the correct key.
		img := newTestImageTag("1.0.21", imgManifestDigest)
		wrongKP := newTestKeyPair(t)
		bundleManifest, blobDigest, blobBytes := makeDSSEBundleWithSigners(t, []*ecdsa.PrivateKey{wrongKP.priv, kp.priv}, imgManifestDigest)

		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		err := VerifyWithPublicKey(ctx, img, verifyConfig, fetcher)
		assert.NoError(t, err)
	})

	t.Run("tag-based fallback verified end-to-end when OCI referrers API is unavailable", func(t *testing.T) {
		// Models a real-world local registry (e.g. Docker Distribution v2) that
		// returns 404 for the Referrers endpoint; cosign stored the signature as
		// the tag "sha256-<hex>.sig".
		const fallbackTag = "sha256-ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234.sig"
		img := newTestImageTag("1.0.21", imgManifestDigest)
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)

		fetcher := &mockFetcher{
			referrerErrors: map[string]error{
				imgManifestDigest: fmt.Errorf("404 page not found"),
			},
			manifests: map[string]distribution.Manifest{
				fallbackTag: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		err := VerifyWithPublicKey(ctx, img, verifyConfig, fetcher)
		assert.NoError(t, err)
		require.NotEmpty(t, img.ImageTag.TagSignatures)
	})

	t.Run("tag-based OCI Image Index fallback verified end-to-end (cosign 2.x registry)", func(t *testing.T) {
		// cosign 2.x stores signatures as an OCI Image Index at "sha256-<hex>.sig".
		const fallbackTag = "sha256-ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234.sig"
		img := newTestImageTag("1.0.21", imgManifestDigest)
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, imgManifestDigest)

		fetcher := &mockFetcher{
			referrerErrors: map[string]error{
				imgManifestDigest: fmt.Errorf("404 page not found"),
			},
			manifests: map[string]distribution.Manifest{
				fallbackTag: &mockIndex{
					refs: []distribution.Descriptor{
						{Digest: godigest.Digest(sigArtifactDigest)},
					},
				},
				sigArtifactDigest: bundleManifest,
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		err := VerifyWithPublicKey(ctx, img, verifyConfig, fetcher)
		assert.NoError(t, err)
		require.NotEmpty(t, img.ImageTag.TagSignatures)
	})
}
