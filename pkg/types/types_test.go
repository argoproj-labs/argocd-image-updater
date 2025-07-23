package types

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
)

func Test_GetParameterPullSecret(t *testing.T) {
	t.Run("Get cred source from a valid pull secret string", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("dummy=foo/bar:1.12"))
		img.PullSecret = "pullsecret:foo/bar"
		credSrc := img.GetParameterPullSecret(context.Background())
		require.NotNil(t, credSrc)
		assert.Equal(t, image.CredentialSourcePullSecret, credSrc.Type)
		assert.Equal(t, "foo", credSrc.SecretNamespace)
		assert.Equal(t, "bar", credSrc.SecretName)
		assert.Equal(t, ".dockerconfigjson", credSrc.SecretField)
	})

	t.Run("Return nil for an invalid pull secret string", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("dummy=foo/bar:1.12"))
		img.PullSecret = "pullsecret:invalid"
		credSrc := img.GetParameterPullSecret(context.Background())
		require.Nil(t, credSrc)
	})

	t.Run("Return nil for an empty pull secret string", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("dummy=foo/bar:1.12"))
		// img.PullSecret is "" by default, so no need to set it
		credSrc := img.GetParameterPullSecret(context.Background())
		require.Nil(t, credSrc)
	})
}
