package registry

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"
	"github.com/argoproj/pkg/json"

	"github.com/distribution/distribution/v3"
	"github.com/distribution/distribution/v3/manifest/ocischema"
	"github.com/distribution/distribution/v3/manifest/schema1"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/distribution/distribution/v3/reference"
	"github.com/distribution/distribution/v3/registry/client"
	"github.com/distribution/distribution/v3/registry/client/auth"
	"github.com/distribution/distribution/v3/registry/client/auth/challenge"
	"github.com/distribution/distribution/v3/registry/client/transport"

	"github.com/opencontainers/go-digest"

	"go.uber.org/ratelimit"

	"net/http"
	"net/url"
	"strings"
	"time"
)

// TODO: Check image's architecture and OS

// RegistryClient defines the methods we need for querying container registries
type RegistryClient interface {
	NewRepository(nameInRepository string) error
	Tags() ([]string, error)
	Manifest(tagStr string)(distribution.Manifest, error)
	TagMetadata(manifest distribution.Manifest) (*tag.TagInfo, error)
}

type NewRegistryClient func(*RegistryEndpoint, string, string) (RegistryClient, error)

// Helper type for registry clients
type registryClient struct {
	regClient distribution.Repository
	endpoint *RegistryEndpoint
	name   reference.Named
	creds credentials
}
// credentials is an implementation of distribution/V3/session struct
// to mangage registry credentials and token
type credentials struct {
	username      string
	password      string
	refreshTokens map[string]string
}

func (c credentials) Basic(url *url.URL) (string, string) {
	return c.username, c.password
}

func (c credentials) RefreshToken(url *url.URL, service string) string {
	return c.refreshTokens[service]
}

func (c credentials) SetRefreshToken(realm *url.URL, service, token string) {
	if c.refreshTokens != nil {
		c.refreshTokens[service] = token
	}
}

// rateLimitTransport encapsulates our custom HTTP round tripper with rate
// limiter from the endpoint.
type rateLimitTransport struct {
	limiter   ratelimit.Limiter
	transport http.RoundTripper
	endpoint  string
}

// RoundTrip is a custom RoundTrip method with rate-limiter
func (rlt *rateLimitTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rlt.limiter.Take()
	log.Tracef("%s", r.URL)
	resp, err := rlt.transport.RoundTrip(r)
	metrics.Endpoint().IncreaseRequest(rlt.endpoint, err != nil)
	return resp, err
}

// NewRepository is a wrapper for creating a registry client that is possibly
// rate-limited by using a custom HTTP round tripper method.
func (clt *registryClient)NewRepository(nameInRepository string) error {
	urlToCall := strings.TrimSuffix(clt.endpoint.RegistryAPI, "/")
	challengeManager1 := challenge.NewSimpleManager()
	_, err := ping(challengeManager1, clt.endpoint.RegistryAPI+"/v2/", "")
	if err != nil {
		return err
	}
	var transport http.RoundTripper = transport.NewTransport(
		nil, auth.NewAuthorizer(
			challengeManager1,
			auth.NewTokenHandler(nil, clt.creds, nameInRepository, "pull")))

	rlt := &rateLimitTransport{
		limiter:   clt.endpoint.Limiter,
		transport: transport,
		endpoint:  clt.endpoint.RegistryAPI,
	}

	named,err := reference.WithName(nameInRepository)
	if err != nil {
		return err
	}
	clt.regClient, err = client.NewRepository(named,urlToCall, rlt)
	if err != nil {
		return err
	}
	return nil
}

// NewClient returns a new RegistryClient for the given endpoint information
func NewClient(endpoint *RegistryEndpoint, username, password string) (RegistryClient, error) {
	if username == "" && endpoint.Username != "" {
		username = endpoint.Username
	}
	if password == "" && endpoint.Password != "" {
		password = endpoint.Password
	}
	creds := credentials{
		username: username,
		password: password,
	}
	return &registryClient{
		creds: creds,
		endpoint: endpoint,
	}, nil
}

// Tags returns a list of tags for given name in repository
func (clt *registryClient) Tags() ([]string, error) {
	tagService := clt.regClient.Tags(context.Background())
	tTags,err := tagService.All(context.Background())
	if err != nil {
		return nil, err
	}
	return tTags,nil
}

// Manifest  returns a Manifest for a given tag in repository
func (clt *registryClient) Manifest(tagStr string) (distribution.Manifest, error) {
	manService, err := clt.regClient.Manifests(context.Background())
	if err != nil {
		return nil, err
	}
	mediaType  := []string{ocischema.SchemaVersion.MediaType, schema1.SchemaVersion.MediaType, schema2.SchemaVersion.MediaType}
	manifest,err := manService.Get(
		context.Background(),
		digest.FromString(tagStr),
		distribution.WithTag(tagStr), distribution.WithManifestMediaTypes(mediaType))
	if err != nil {
		return nil, err
	}
	return manifest,nil
}

// TagMetadata retrieves metadata for a given manifest of given repository
func (client *registryClient) TagMetadata(manifest distribution.Manifest) (*tag.TagInfo, error) {
	ti := &tag.TagInfo{}

	var info struct {
		Arch    string `json:"architecture"`
		Created string `json:"created"`
		OS      string `json:"os"`
	}
	//
	// We support both V1,V2 AND OCI manifest schemas. Everything else will trigger
	// an error.
	switch deserialized := manifest.(type) {

	case *schema1.SignedManifest:
		var man schema1.Manifest = deserialized.Manifest
		if len(man.History) == 0 {
			return nil, fmt.Errorf("no history information found in schema V1")
		}
		if err := json.Unmarshal([]byte(man.History[0].V1Compatibility), &info); err != nil {
			return nil, err
		}
		if createdAt, err := time.Parse(time.RFC3339Nano, info.Created); err != nil {
			return nil, err
		} else {
			ti.CreatedAt = createdAt
		}
		_, mBytes, err := manifest.Payload()
		if err != nil {
			return nil, err
		}
		ti.Digest = sha256.Sum256(mBytes)
		log.Tracef("v1 SHA digest is %s", fmt.Sprintf("sha256:%x", ti.Digest))
		return ti, nil

	case *schema2.DeserializedManifest:
		var man schema2.Manifest = deserialized.Manifest

		// The data we require from a V2 manifest is in a blob that we need to
		// fetch from the registry.
		blobReader, err := client.regClient.Blobs(context.Background()).Get(context.Background(), man.Config.Digest)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(blobReader, &info); err != nil {
			return nil, err
		}

		if ti.CreatedAt, err = time.Parse(time.RFC3339Nano, info.Created); err != nil {
			return nil, err
		}

		_, mBytes, err := manifest.Payload()
		if err != nil {
			return nil, err
		}
		ti.Digest = sha256.Sum256(mBytes)
		log.Tracef("v2 SHA digest is %s", fmt.Sprintf("sha256:%x", ti.Digest))
		return ti, nil
	case *ocischema.DeserializedManifest:
		var man ocischema.Manifest = deserialized.Manifest

		// The data we require from a V2 manifest is in a blob that we need to
		// fetch from the registry.
		blobReader, err := client.regClient.Blobs(context.Background()).Get(context.Background(), man.Config.Digest)
		if err != nil {
			return nil, err
		}

		if err := json.Unmarshal(blobReader, &info); err != nil {
			return nil, err
		}

		if ti.CreatedAt, err = time.Parse(time.RFC3339Nano, info.Created); err != nil {
			return nil, err
		}

		_, mBytes, err := manifest.Payload()
		if err != nil {
			return nil, err
		}
		ti.Digest = sha256.Sum256(mBytes)
		log.Tracef("oci SHA digest is %s", fmt.Sprintf("sha256:%x", ti.Digest))
		return ti, nil
	default:
		return nil, fmt.Errorf("invalid manifest type")
	}
}

// Implementation of ping method to intialize the challenge list
// Without this, tokenHandler and AuthorizationHandler won't work
func ping(manager challenge.Manager, endpoint, versionHeader string) ([]auth.APIVersion, error) {
	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := manager.AddResponse(resp); err != nil {
		return nil, err
	}

	return auth.APIVersions(resp, versionHeader), err
}
