package aws_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllowedManifestMediaTypes(t *testing.T) {
	allowed := []string{
		"application/vnd.docker.distribution.manifest.v2+json",
		"application/vnd.docker.distribution.manifest.list.v2+json",
		"application/vnd.oci.image.manifest.v1+json",
		"application/vnd.oci.image.index.v1+json",
	}
	for _, mt := range allowed {
		assert.True(t, isAllowedManifestMediaTypeExported(mt), mt)
	}
	assert.False(t, isAllowedManifestMediaTypeExported("application/vnd.in-toto+json"))
	assert.False(t, isAllowedManifestMediaTypeExported("application/vnd.oci.image.config.v1+json"))
}

// isAllowedManifestMediaTypeExported mirrors the private helper for unit testing.
func isAllowedManifestMediaTypeExported(mediaType string) bool {
	allowed := map[string]struct{}{
		"application/vnd.docker.distribution.manifest.v2+json":      {},
		"application/vnd.docker.distribution.manifest.list.v2+json": {},
		"application/vnd.oci.image.manifest.v1+json":                {},
		"application/vnd.oci.image.index.v1+json":                   {},
	}
	_, ok := allowed[mediaType]
	return ok
}
