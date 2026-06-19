package argocd

import (
	"context"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/aws"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

// isStaleEventDrivenCandidate reports whether an event-driven candidate should be
// ignored because the application already references an image that is the same
// or was pushed more recently.
func isStaleEventDrivenCandidate(
	ctx context.Context,
	updateConf *UpdateConfiguration,
	appImages *ApplicationImages,
	applicationImage *Image,
	rep *registry.RegistryEndpoint,
	candidate *tag.ImageTag,
) (bool, error) {
	if updateConf == nil || updateConf.WebhookEvent == nil || candidate == nil {
		return false, nil
	}

	currentImageStr, err := getAppImage(ctx, &appImages.Application, appImages.WriteBackConfig, applicationImage)
	if err != nil {
		return false, err
	}
	if currentImageStr == "" {
		return false, nil
	}

	currentImg := image.NewFromIdentifier(currentImageStr)
	currentTag := currentImg.ImageTag
	if currentTag == nil || currentTag.TagName == "" {
		return false, nil
	}

	if candidate.TagDigest != "" && currentTag.TagDigest != "" && candidate.TagDigest == currentTag.TagDigest {
		return true, nil
	}

	currentPushedAt, err := resolveImagePushedAt(ctx, updateConf, applicationImage, rep, currentTag.TagName)
	if err != nil {
		return false, err
	}

	return eventCandidateIsNotNewerThan(candidate, currentTag, currentPushedAt), nil
}

// eventCandidateIsNotNewerThan returns true when the candidate should not replace
// the current application image based on digest and push time.
func eventCandidateIsNotNewerThan(candidate *tag.ImageTag, currentTag *tag.ImageTag, currentPushedAt time.Time) bool {
	if candidate == nil {
		return false
	}

	if candidate.TagDigest != "" && currentTag != nil && currentTag.TagDigest != "" && candidate.TagDigest == currentTag.TagDigest {
		return true
	}

	if candidate.TagDate == nil || candidate.TagDate.IsZero() {
		return false
	}
	if currentPushedAt.IsZero() {
		return false
	}

	return !candidate.TagDate.After(currentPushedAt)
}

func resolveImagePushedAt(
	ctx context.Context,
	updateConf *UpdateConfiguration,
	applicationImage *Image,
	rep *registry.RegistryEndpoint,
	tagName string,
) (time.Time, error) {
	registryURL := applicationImage.ContainerImage.RegistryURL
	if registryURL == "" && rep != nil {
		registryURL = rep.RegistryAPI
	}
	if !aws.IsECRRegistryURL(registryURL) {
		return time.Time{}, nil
	}

	region := updateConf.AWSRegion
	if region == "" {
		_, parsedRegion, ok := aws.ParseECRRegistryURL(registryURL)
		if !ok {
			return time.Time{}, nil
		}
		region = parsedRegion
	}

	imgTag, err := aws.DescribeImageTag(ctx, aws.ClientConfig{
		Region:      region,
		EndpointURL: updateConf.AWSEndpointURL,
	}, applicationImage.ContainerImage.ImageName, tagName)
	if err != nil {
		log.LoggerFromContext(ctx).Warnf("Could not resolve push time for current image tag %q: %v", tagName, err)
		return time.Time{}, nil
	}
	if imgTag.TagDate == nil {
		return time.Time{}, nil
	}
	return *imgTag.TagDate, nil
}
