package argocd

import (
	"context"
	"sync"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/aws"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

type appliedEvent struct {
	tag      string
	pushedAt time.Time
}

// EventFreshnessStore records event-driven writes so out-of-order SQS messages
// can be detected without relying on a fresh Application informer view.
type EventFreshnessStore struct {
	mu              sync.RWMutex
	tagPushTimes    map[string]time.Time
	latestByRepo    map[string]appliedEvent
}

// NewEventFreshnessStore returns an empty freshness store.
func NewEventFreshnessStore() *EventFreshnessStore {
	return &EventFreshnessStore{
		tagPushTimes: make(map[string]time.Time),
		latestByRepo: make(map[string]appliedEvent),
	}
}

func tagPushTimeKey(repository, tag string) string {
	return repository + "/" + tag
}

// RecordApplied stores push metadata for a successfully applied event-driven update.
func (s *EventFreshnessStore) RecordApplied(repository, tag string, pushedAt time.Time) {
	if s == nil || repository == "" || tag == "" || pushedAt.IsZero() {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.tagPushTimes[tagPushTimeKey(repository, tag)] = pushedAt
	s.latestByRepo[repository] = appliedEvent{tag: tag, pushedAt: pushedAt}
}

// TagPushTime returns a previously recorded push time for a repository tag.
func (s *EventFreshnessStore) TagPushTime(repository, tag string) (time.Time, bool) {
	if s == nil || repository == "" || tag == "" {
		return time.Time{}, false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	pushedAt, ok := s.tagPushTimes[tagPushTimeKey(repository, tag)]
	return pushedAt, ok
}

// IsOutOfOrder reports whether an event for a different tag is older than the
// last event-driven update recorded for the repository.
func (s *EventFreshnessStore) IsOutOfOrder(repository, eventTag string, eventPushedAt time.Time) bool {
	if s == nil || repository == "" || eventTag == "" || eventPushedAt.IsZero() {
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	latest, ok := s.latestByRepo[repository]
	if !ok || latest.tag == eventTag {
		return false
	}
	return !eventPushedAt.After(latest.pushedAt)
}

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

	event := updateConf.WebhookEvent
	if updateConf.EventFreshness != nil && !event.PushedAt.IsZero() &&
		updateConf.EventFreshness.IsOutOfOrder(applicationImage.ContainerImage.ImageName, event.Tag, event.PushedAt) {
		return true, nil
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

	currentPushedAt, err := resolveCurrentTagPushTime(ctx, updateConf, applicationImage, rep, currentTag.TagName)
	if err != nil {
		return false, err
	}

	eventPushedAt := time.Time{}
	if !event.PushedAt.IsZero() {
		eventPushedAt = event.PushedAt.UTC()
	}

	return eventIsNotNewerThan(candidate, currentTag, currentPushedAt, eventPushedAt), nil
}

// eventIsNotNewerThan returns true when the event candidate should not replace
// the current application image based on digest and push time.
func eventIsNotNewerThan(candidate *tag.ImageTag, currentTag *tag.ImageTag, currentPushedAt, eventPushedAt time.Time) bool {
	if candidate == nil {
		return false
	}

	candidatePushedAt := eventPushedAt
	if candidatePushedAt.IsZero() && candidate.TagDate != nil {
		candidatePushedAt = candidate.TagDate.UTC()
	}
	if candidatePushedAt.IsZero() || currentPushedAt.IsZero() {
		return false
	}

	return !candidatePushedAt.After(currentPushedAt)
}

func resolveCurrentTagPushTime(
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
		if updateConf.EventFreshness != nil {
			if pushedAt, ok := updateConf.EventFreshness.TagPushTime(applicationImage.ContainerImage.ImageName, tagName); ok {
				return pushedAt, nil
			}
		}
		return time.Time{}, nil
	}
	if imgTag.TagDate == nil {
		return time.Time{}, nil
	}
	return imgTag.TagDate.UTC(), nil
}

func recordEventDrivenWrite(updateConf *UpdateConfiguration, changeList []ChangeEntry) {
	if updateConf == nil || updateConf.WebhookEvent == nil || updateConf.EventFreshness == nil || len(changeList) == 0 {
		return
	}

	for _, entry := range changeList {
		if entry.NewTag == nil || entry.Image == nil {
			continue
		}
		pushedAt := updateConf.WebhookEvent.PushedAt.UTC()
		if pushedAt.IsZero() && entry.NewTag.TagDate != nil && !entry.NewTag.TagDate.IsZero() {
			pushedAt = entry.NewTag.TagDate.UTC()
		}
		if pushedAt.IsZero() {
			continue
		}
		updateConf.EventFreshness.RecordApplied(entry.Image.ImageName, entry.NewTag.TagName, pushedAt)
	}
}
