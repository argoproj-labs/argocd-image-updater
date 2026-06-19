package aws

import "errors"

var (
	// ErrMissingRegion is returned when AWS region is not configured.
	ErrMissingRegion = errors.New("aws region is required")
	// ErrSkippedMediaType is returned when an image push is not a container image manifest.
	ErrSkippedMediaType = errors.New("image manifest media type is not a supported container image")
	// ErrSkippedEvent is returned for EventBridge events that should be ignored.
	ErrSkippedEvent = errors.New("event is not a supported ECR image push")
)
