package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecr"
	ecrtypes "github.com/aws/aws-sdk-go-v2/service/ecr/types"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

var allowedManifestMediaTypes = map[string]struct{}{
	"application/vnd.docker.distribution.manifest.v2+json":      {},
	"application/vnd.docker.distribution.manifest.list.v2+json": {},
	"application/vnd.oci.image.manifest.v1+json":                {},
	"application/vnd.oci.image.index.v1+json":                   {},
}

func isAllowedManifestMediaType(mediaType string) bool {
	_, ok := allowedManifestMediaTypes[mediaType]
	return ok
}

// DescribeImageTag fetches digest and push time for a single ECR image tag.
func DescribeImageTag(ctx context.Context, cfg ClientConfig, repositoryName, imageTag string) (*tag.ImageTag, error) {
	awsCfg, err := loadAWSConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	client := ecr.NewFromConfig(awsCfg)
	out, err := client.DescribeImages(ctx, &ecr.DescribeImagesInput{
		RepositoryName: &repositoryName,
		ImageIds: []ecrtypes.ImageIdentifier{
			{ImageTag: &imageTag},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("ecr DescribeImages failed for %s:%s: %w", repositoryName, imageTag, err)
	}
	if len(out.ImageDetails) == 0 {
		return nil, fmt.Errorf("ecr DescribeImages returned no images for %s:%s", repositoryName, imageTag)
	}

	detail := out.ImageDetails[0]
	if detail.ImageManifestMediaType != nil && !isAllowedManifestMediaType(*detail.ImageManifestMediaType) {
		return nil, fmt.Errorf("%w: %s", ErrSkippedMediaType, *detail.ImageManifestMediaType)
	}

	digest := ""
	if detail.ImageDigest != nil {
		digest = NormalizeDigest(*detail.ImageDigest)
	}

	pushedAt := time.Time{}
	if detail.ImagePushedAt != nil {
		pushedAt = *detail.ImagePushedAt
	}

	return tag.NewImageTag(imageTag, pushedAt, digest), nil
}

// NormalizeDigest ensures a sha256 digest has the standard prefix.
func NormalizeDigest(digest string) string {
	if strings.HasPrefix(digest, "sha256:") {
		return digest
	}
	return "sha256:" + digest
}
