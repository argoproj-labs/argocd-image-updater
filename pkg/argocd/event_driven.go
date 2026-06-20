package argocd

import (
	"context"
	"errors"

	"github.com/argoproj-labs/argocd-image-updater/pkg/aws"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

type eventDrivenTagsResult struct {
	tags    *tag.ImageTagList
	skipped bool
	err     error
}

func resolveEventDrivenTags(
	ctx context.Context,
	updateConf *UpdateConfiguration,
	applicationImage *Image,
	rep *registry.RegistryEndpoint,
	vc *image.VersionConstraint,
) eventDrivenTagsResult {
	if updateConf.WebhookEvent == nil || !webhookMatchesConfiguredImage(updateConf.WebhookEvent, applicationImage.ContainerImage) {
		return eventDrivenTagsResult{}
	}

	imgCtx := log.LoggerFromContext(ctx)
	eventTag := updateConf.WebhookEvent.Tag
	if (vc.MatchFunc != nil && !vc.MatchFunc(eventTag, vc.MatchArgs)) || vc.IsTagIgnored(ctx, eventTag) {
		imgCtx.Debugf("Event tag %q did not pass allow/ignore filters, skipping", eventTag)
		return eventDrivenTagsResult{skipped: true}
	}

	registryURL := applicationImage.ContainerImage.RegistryURL
	if registryURL == "" {
		registryURL = rep.RegistryAPI
	}
	if !aws.IsECRRegistryURL(registryURL) {
		vc.ExactTag = eventTag
		return eventDrivenTagsResult{}
	}

	region := updateConf.AWSRegion
	if region == "" {
		if _, parsedRegion, ok := aws.ParseECRRegistryURL(registryURL); ok {
			region = parsedRegion
		}
	}

	imgTag, err := aws.ResolveEventTag(ctx, aws.ResolveEventTagInput{
		ClientConfig: aws.ClientConfig{
			Region:      region,
			EndpointURL: updateConf.AWSEndpointURL,
		},
		Repository:              applicationImage.ContainerImage.ImageName,
		Event:                   pushEventFromWebhook(updateConf.WebhookEvent),
		FallbackOnDescribeError: updateConf.ECRFallbackOnDescribeError,
	})
	if errors.Is(err, aws.ErrSkippedMediaType) {
		imgCtx.Infof("Skipping event-driven update for unsupported manifest media type: %v", err)
		return eventDrivenTagsResult{skipped: true}
	}
	if err != nil {
		return eventDrivenTagsResult{err: err}
	}

	tags := tag.NewImageTagList()
	tags.Add(imgTag)
	imgCtx.Debugf("Using ECR DescribeImages for event-driven tag %q, skipping repository tag listing", eventTag)
	return eventDrivenTagsResult{tags: tags}
}

func skipIfStaleEventDrivenUpdate(
	ctx context.Context,
	updateConf *UpdateConfiguration,
	applicationImage *Image,
	rep *registry.RegistryEndpoint,
	candidate *tag.ImageTag,
) (bool, error) {
	if updateConf.WebhookEvent == nil || !webhookMatchesConfiguredImage(updateConf.WebhookEvent, applicationImage.ContainerImage) {
		return false, nil
	}
	return isStaleEventDrivenCandidate(ctx, updateConf, updateConf.UpdateApp, applicationImage, rep, candidate)
}

func pushEventFromWebhook(event *WebhookEvent) aws.PushEventInfo {
	if event == nil {
		return aws.PushEventInfo{}
	}
	return aws.PushEventInfo{
		Tag:      event.Tag,
		Digest:   event.Digest,
		PushedAt: event.PushedAt,
	}
}
