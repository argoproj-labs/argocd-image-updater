package aws

import "time"

// ImagePushEvent is a normalized container image push event from AWS EventBridge.
type ImagePushEvent struct {
	RegistryURL string
	Repository  string
	Tag         string
	Digest      string
	PushedAt    time.Time
}
