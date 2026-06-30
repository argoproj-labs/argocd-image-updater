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

// makeDSSEBundle creates a properly signed sigstore bundle, together with its
// OCI manifest. Returns the manifest, the blob digest (for mockFetcher.blobs
// keying), and the raw blob bytes.
func makeDSSEBundle(t *testing.T, priv *ecdsa.PrivateKey, payload []byte) (manifest *ocischema.DeserializedManifest, blobDigest string, blobBytes []byte) {
	t.Helper()

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
// fetchTagSignature
// ---------------------------------------------------------------------------

func Test_fetchTagSignature(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)
	payload := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:29d4b"}}}`)

	const (
		imgManifestDigest = "sha256:aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234aabb1234"
		sigArtifactDigest = "sha256:ccdd5678ccdd5678ccdd5678ccdd5678ccdd5678ccdd5678ccdd5678ccdd5678"
	)

	t.Run("fast path uses ManifestDigest and succeeds", func(t *testing.T) {
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, payload)
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

		got, err := fetchTagSignature(ctx, imgTag, fetcher)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got, kp.pemPub))
	})

	t.Run("slow path fetches image manifest when ManifestDigest is empty", func(t *testing.T) {
		imgPayload := []byte(`{"schemaVersion":2}`)
		imgDigest := godigest.FromBytes(imgPayload).String()
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, payload)

		imgTag := &tag.ImageTag{TagName: "1.0.21"} // no ManifestDigest
		fetcher := &mockFetcher{
			manifests: map[string]distribution.Manifest{
				"1.0.21":            &mockManifest{payload: imgPayload},
				sigArtifactDigest:  bundleManifest,
			},
			referrers: map[string][]distribution.Descriptor{
				imgDigest: {bundleReferrer(sigArtifactDigest)},
			},
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := fetchTagSignature(ctx, imgTag, fetcher)
		require.NoError(t, err)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got, kp.pemPub))
	})

	t.Run("invalid ManifestDigest format returns error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: "not-a-digest"}
		_, err := fetchTagSignature(ctx, imgTag, &mockFetcher{})
		assert.ErrorContains(t, err, "invalid manifest digest")
	})

	t.Run("Referrers error returns error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrerErrors: map[string]error{
				imgManifestDigest: fmt.Errorf("registry unavailable"),
			},
		}
		_, err := fetchTagSignature(ctx, imgTag, fetcher)
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
		_, err := fetchTagSignature(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers")
	})

	t.Run("empty referrers returns error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		_, err := fetchTagSignature(ctx, imgTag, &mockFetcher{})
		assert.ErrorContains(t, err, "no cosign signature found in OCI referrers")
	})

	t.Run("ManifestForDigest error propagates", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			errors: map[string]error{
				sigArtifactDigest: fmt.Errorf("network timeout"),
			},
		}
		_, err := fetchTagSignature(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "error fetching cosign signature manifest")
	})

	t.Run("sig manifest is not an OCI manifest returns error", func(t *testing.T) {
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
		_, err := fetchTagSignature(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "not an OCI image manifest")
	})

	t.Run("sig manifest has no layers returns error", func(t *testing.T) {
		imgTag := &tag.ImageTag{TagName: "1.0.21", ManifestDigest: imgManifestDigest}
		fetcher := &mockFetcher{
			referrers: map[string][]distribution.Descriptor{
				imgManifestDigest: {bundleReferrer(sigArtifactDigest)},
			},
			manifests: map[string]distribution.Manifest{
				sigArtifactDigest: &ocischema.DeserializedManifest{},
			},
		}
		_, err := fetchTagSignature(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "no layers in signature manifest")
	})

	t.Run("layer has wrong media type returns error", func(t *testing.T) {
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
		_, err := fetchTagSignature(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "unsupported cosign layer media type")
	})

	t.Run("blob fetch error returns error", func(t *testing.T) {
		bundleManifest, blobDigest, _ := makeDSSEBundle(t, kp.priv, payload)
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
		_, err := fetchTagSignature(ctx, imgTag, fetcher)
		assert.ErrorContains(t, err, "failed to fetch sigstore bundle blob")
	})
}

// ---------------------------------------------------------------------------
// extractSigFromManifest
// ---------------------------------------------------------------------------

func Test_extractSigFromManifest(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)
	payload := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:abc"}}}`)

	const manifestRef = "sha256:aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000aaaa0000"

	t.Run("not an OCI manifest returns error", func(t *testing.T) {
		_, err := extractSigFromManifest(ctx, &mockManifest{}, manifestRef, "1.0.21", &mockFetcher{})
		assert.ErrorContains(t, err, "not an OCI image manifest")
	})

	t.Run("no layers returns error", func(t *testing.T) {
		m := &ocischema.DeserializedManifest{} // zero layers
		_, err := extractSigFromManifest(ctx, m, manifestRef, "1.0.21", &mockFetcher{})
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
		_, err := extractSigFromManifest(ctx, m, manifestRef, "1.0.21", &mockFetcher{})
		assert.ErrorContains(t, err, "unsupported cosign layer media type")
	})

	t.Run("valid bundle layer delegates to extractDSSEBundle and succeeds", func(t *testing.T) {
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, payload)
		// bundleManifest is an *ocischema.DeserializedManifest with sigstoreBundleType layer.
		fetcher := &mockFetcher{
			blobs: map[string][]byte{blobDigest: blobBytes},
		}

		got, err := extractSigFromManifest(ctx, bundleManifest, manifestRef, "1.0.21", fetcher)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got, kp.pemPub))
	})
}

// ---------------------------------------------------------------------------
// extractDSSEBundle
// ---------------------------------------------------------------------------

func Test_extractDSSEBundle(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)
	payload := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:29d4b"}}}`)
	payloadType := "application/vnd.dev.cosign.simplesigning.v1+json"

	// makeLayer is a helper that builds a distribution.Descriptor pointing at
	// specific blob bytes (or nothing if blobDigest is empty).
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
		_, err := extractDSSEBundle(ctx, layer, "1.0.21", fetcher)
		assert.ErrorContains(t, err, "failed to fetch sigstore bundle blob")
	})

	t.Run("invalid JSON in blob returns error", func(t *testing.T) {
		blobBytes := []byte(`not-json`)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err := extractDSSEBundle(ctx, layer, "1.0.21", fetcher)
		assert.ErrorContains(t, err, "failed to parse sigstore bundle")
	})

	t.Run("bundle has no dsseEnvelope returns error", func(t *testing.T) {
		blobBytes, err := json.Marshal(sigstoreBundle{}) // DSSEEnvelope is nil
		require.NoError(t, err)
		layer := makeLayer(blobBytes)
		fetcher := &mockFetcher{
			blobs: map[string][]byte{layer.Digest.String(): blobBytes},
		}
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", fetcher)
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
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", fetcher)
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
		_, err = extractDSSEBundle(ctx, layer, "1.0.21", fetcher)
		assert.ErrorContains(t, err, "failed to decode dsseEnvelope payload")
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

		got, err := extractDSSEBundle(ctx, layer, "1.0.21", fetcher)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, hex.EncodeToString(paeHash[:]), got.PayloadDigest)
		assert.NoError(t, verifySignature("quay.io/org/app:1.0.21", got, kp.pemPub))
	})
}

// ---------------------------------------------------------------------------
// VerifyWithPublicKey
// ---------------------------------------------------------------------------

func Test_VerifyWithPublicKey(t *testing.T) {
	ctx := context.Background()
	kp := newTestKeyPair(t)
	payload := []byte(`{"critical":{"image":{"docker-manifest-digest":"sha256:29d4b"}}}`)

	const (
		imgManifestDigest = "sha256:ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234ccdd1234"
		sigArtifactDigest = "sha256:eeff5678eeff5678eeff5678eeff5678eeff5678eeff5678eeff5678eeff5678"
	)

	verifyConfig := &Verify{Method: "cosign-key", PublicKeySecret: kp.pemPub}

	t.Run("nil ImageTag returns error", func(t *testing.T) {
		img := &ContainerImage{RegistryURL: "quay.io", ImageName: "org/app"}
		err := VerifyWithPublicKey(ctx, img, verifyConfig, &mockFetcher{})
		assert.ErrorContains(t, err, "no tag information")
	})

	t.Run("uses cached TagSignature without any network call", func(t *testing.T) {
		img := newTestImageTag("1.0.21", imgManifestDigest)
		payloadType := "application/vnd.dev.cosign.simplesigning.v1+json"
		digestHex, sigB64 := signPAE(t, kp.priv, payloadType, payload)
		img.ImageTag.TagSignature = &tag.TagSignature{Sig: sigB64, PayloadDigest: digestHex}

		err := VerifyWithPublicKey(ctx, img, verifyConfig, &mockFetcher{})
		assert.NoError(t, err)
	})

	t.Run("DSSE bundle verifies successfully end-to-end", func(t *testing.T) {
		img := newTestImageTag("1.0.21", imgManifestDigest)
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, kp.priv, payload)

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
		// TagSignature is cached on the tag after fetch.
		require.NotNil(t, img.ImageTag.TagSignature)
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
		bundleManifest, blobDigest, blobBytes := makeDSSEBundle(t, otherKP.priv, payload)

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
}
