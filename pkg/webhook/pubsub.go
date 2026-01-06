package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/pubsub/v2"
	"github.com/sirupsen/logrus"
	"google.golang.org/api/option"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// ArtifactRegistryWebhook handles Google Artifact Registry webhook events via Pub/Sub push.
// This implements the RegistryWebhook interface for Pub/Sub HTTP push notifications.
// See: https://cloud.google.com/pubsub/docs/push
// See: https://cloud.google.com/artifact-registry/docs/configure-notifications
type ArtifactRegistryWebhook struct {
	secret string
}

// NewArtifactRegistryWebhook creates a new Artifact Registry webhook handler
func NewArtifactRegistryWebhook(secret string) *ArtifactRegistryWebhook {
	return &ArtifactRegistryWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the registry type this handler supports
func (a *ArtifactRegistryWebhook) GetRegistryType() string {
	return "artifact-registry"
}

// Validate validates the Artifact Registry webhook payload
func (a *ArtifactRegistryWebhook) Validate(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// If secret is configured, validate it via query parameter.
	// Note: Pub/Sub can be configured to add an Authorization header (OIDC), but
	// Image Updater currently relies on the query parameter secret for validation.
	if a.secret != "" {
		secret := r.URL.Query().Get("secret")
		if secret == "" {
			return fmt.Errorf("missing webhook secret")
		} else if secret != a.secret {
			return fmt.Errorf("invalid webhook secret")
		}
	}

	return nil
}

// PubSubPushMessage represents the Pub/Sub push message envelope.
// See: https://cloud.google.com/pubsub/docs/push#receive_push
type PubSubPushMessage struct {
	Message struct {
		Data        string `json:"data"` // Base64-encoded
		MessageID   string `json:"messageId"`
		PublishTime string `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

type pubSubDisposition int

const (
	pubSubAck pubSubDisposition = iota
	pubSubNack
)

type ignoredWebhookError struct {
	reason error
}

var (
	ErrNonInsertAction  = errors.New("non-INSERT action")
	ErrEmptyMessageData = errors.New("empty message data")
)

func (e *ignoredWebhookError) Error() string {
	if e == nil || e.reason == nil {
		return ErrWebhookIgnored.Error()
	}
	return e.reason.Error()
}

func (e *ignoredWebhookError) Unwrap() error {
	return ErrWebhookIgnored
}

func decodePubSubPushRequestData(body []byte) ([]byte, error) {
	// Pub/Sub push can deliver messages wrapped (default) or unwrapped (payload unwrapping).
	// Wrapped: JSON envelope with base64 message.data.
	// Unwrapped: raw message data as the HTTP body.
	// See: https://docs.cloud.google.com/pubsub/docs/push
	// Try wrapped format first.
	data, err := decodePubSubPushMessageData(body)
	if err == nil {
		if data != nil {
			return data, nil
		}
		// JSON that isn't a Pub/Sub envelope (no message.data) -> treat as unwrapped.
		return body, nil
	}

	// If it isn't a valid Pub/Sub push envelope (including non-JSON), treat it as unwrapped.
	var parseErr *pubSubPushEnvelopeParseError
	if errors.As(err, &parseErr) {
		return body, nil
	}

	// Anything else (e.g., invalid base64 in message.data) should surface as an error.
	return nil, err
}

type pubSubPushEnvelopeParseError struct {
	err error
}

func (e *pubSubPushEnvelopeParseError) Error() string {
	if e == nil || e.err == nil {
		return "failed to parse Pub/Sub push envelope"
	}
	return fmt.Sprintf("failed to parse Pub/Sub push envelope: %v", e.err)
}

func (e *pubSubPushEnvelopeParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func decodePubSubPushMessageData(body []byte) ([]byte, error) {
	var pushMsg PubSubPushMessage
	if err := json.Unmarshal(body, &pushMsg); err != nil {
		return nil, &pubSubPushEnvelopeParseError{err: err}
	}

	if pushMsg.Message.Data == "" {
		return nil, nil
	}

	data, err := base64.StdEncoding.DecodeString(pushMsg.Message.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 message data: %w", err)
	}

	return data, nil
}

func artifactRegistryEventFromPubSubData(data []byte) (*argocd.WebhookEvent, pubSubDisposition, error) {
	event, err := ParseArtifactRegistryMessage(data)
	if err != nil {
		if isArtifactRegistryIgnorableError(err) {
			return nil, pubSubAck, err
		}
		return nil, pubSubNack, err
	}
	return event, pubSubAck, nil
}

// Parse processes the Artifact Registry webhook payload and returns a WebhookEvent.
// The payload is a Pub/Sub push envelope containing the Artifact Registry notification.
func (a *ArtifactRegistryWebhook) Parse(r *http.Request) (*argocd.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	messageData, err := decodePubSubPushRequestData(body)
	if err != nil {
		return nil, err
	}

	event, disposition, err := artifactRegistryEventFromPubSubData(messageData)
	if err != nil {
		if disposition == pubSubAck {
			return nil, &ignoredWebhookError{reason: err}
		}
		return nil, err
	}

	return event, nil
}

// PubSubSubscriberConfig holds configuration for the Pub/Sub pull subscriber.
type PubSubSubscriberConfig struct {
	// Enabled indicates whether Pub/Sub pull subscriber is enabled
	Enabled bool
	// ProjectID is the GCP project ID containing the subscription
	ProjectID string
	// SubscriptionID is the Pub/Sub subscription ID
	SubscriptionID string
	// CredentialsFile is the path to a service account JSON file (optional, uses ADC if empty)
	CredentialsFile string
	// MaxOutstandingMessages is the maximum number of unprocessed messages
	MaxOutstandingMessages int
	// MaxOutstandingBytes is the maximum size of unprocessed messages in bytes
	MaxOutstandingBytes int
	// NumGoroutines is the number of goroutines to spawn for message processing.
	// GCP recommends 1 for most workloads.
	NumGoroutines int
}

// DefaultPubSubSubscriberConfig returns a PubSubSubscriberConfig with sensible defaults.
func DefaultPubSubSubscriberConfig() *PubSubSubscriberConfig {
	return &PubSubSubscriberConfig{
		Enabled:                false,
		MaxOutstandingMessages: 100,
		MaxOutstandingBytes:    100 * 1024 * 1024, // 100MB
		NumGoroutines:          1,
	}
}

// EventHandler is called when a webhook event is received.
type EventHandler func(ctx context.Context, event *argocd.WebhookEvent) error

// ArtifactRegistrySubscriber is a Pub/Sub pull subscriber for Artifact Registry notifications.
// This implements the pull model for receiving notifications without exposing an HTTP endpoint.
type ArtifactRegistrySubscriber struct {
	config       *PubSubSubscriberConfig
	client       *pubsub.Client
	eventHandler EventHandler
}

// NewArtifactRegistrySubscriber creates a new Artifact Registry Pub/Sub subscriber.
func NewArtifactRegistrySubscriber(cfg *PubSubSubscriberConfig, handler EventHandler) *ArtifactRegistrySubscriber {
	return &ArtifactRegistrySubscriber{
		config:       cfg,
		eventHandler: handler,
	}
}

// Start begins pulling messages from the Pub/Sub subscription.
// It blocks until the context is cancelled.
func (s *ArtifactRegistrySubscriber) Start(ctx context.Context) error {
	logger := log.LoggerFromContext(ctx)
	if logger == nil {
		logger = log.Log().WithFields(logrus.Fields{"logger": "artifact-registry-subscriber"})
	}
	ctx = log.ContextWithLogger(ctx, logger)

	logger.Infof("Starting Artifact Registry Pub/Sub subscriber for project=%s subscription=%s", s.config.ProjectID, s.config.SubscriptionID)

	// Create Pub/Sub client
	var opts []option.ClientOption
	if s.config.CredentialsFile != "" {
		if _, err := os.Stat(s.config.CredentialsFile); err != nil {
			return fmt.Errorf("credentials file not found: %s: %w", s.config.CredentialsFile, err)
		}
		opts = append(opts, option.WithAuthCredentialsFile(option.ServiceAccount, s.config.CredentialsFile))
		logger.Infof("Using credentials from file: %s", s.config.CredentialsFile)
	} else {
		logger.Infof("Using Application Default Credentials (ADC)")
	}

	client, err := pubsub.NewClient(ctx, s.config.ProjectID, opts...)
	if err != nil {
		return fmt.Errorf("failed to create Pub/Sub client: %w", err)
	}
	s.client = client

	defer func() {
		if err := s.client.Close(); err != nil {
			logger.Errorf("Error closing Pub/Sub client: %v", err)
		}
	}()

	// Create subscriber. In Pub/Sub v2 the Exists() method is removed; optimistically
	// expect the subscription to exist and handle NOT_FOUND from Receive.
	sub := s.client.Subscriber(s.config.SubscriptionID)

	// Configure receive settings
	sub.ReceiveSettings.MaxOutstandingMessages = s.config.MaxOutstandingMessages
	sub.ReceiveSettings.MaxOutstandingBytes = s.config.MaxOutstandingBytes
	sub.ReceiveSettings.NumGoroutines = s.config.NumGoroutines

	logger.Infof("Artifact Registry subscriber configured: MaxOutstandingMessages=%d, MaxOutstandingBytes=%d, NumGoroutines=%d",
		s.config.MaxOutstandingMessages, s.config.MaxOutstandingBytes, s.config.NumGoroutines)

	// Start receiving messages
	logger.Infof("Starting to receive messages from subscription %s", s.config.SubscriptionID)
	err = sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		s.handleMessage(ctx, msg)
	})

	if err != nil && ctx.Err() == nil {
		return fmt.Errorf("Pub/Sub receive error: %w", err)
	}

	logger.Infof("Artifact Registry Pub/Sub subscriber stopped")
	return nil
}

// handleMessage processes a single Pub/Sub message.
func (s *ArtifactRegistrySubscriber) handleMessage(ctx context.Context, msg *pubsub.Message) {
	logger := log.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"pubsub_message_id":   msg.ID,
		"pubsub_publish_time": msg.PublishTime.Format(time.RFC3339),
	})
	msgCtx := log.ContextWithLogger(ctx, logger)

	logger.Debugf("Received Pub/Sub message: id=%s", msg.ID)

	// Decode/parse message using the same disposition rules as the push webhook.
	event, disposition, err := artifactRegistryEventFromPubSubData(msg.Data)
	if err != nil {
		if disposition == pubSubAck {
			logger.Infof("Ignoring message: %v", err)
			msg.Ack()
			return
		}

		logger.Errorf("Failed to parse Pub/Sub message: %v", err)
		msg.Nack()
		return
	}

	logger = logger.WithFields(logrus.Fields{
		"event_registry":   event.RegistryURL,
		"event_repository": event.Repository,
		"event_tag":        event.Tag,
		"event_digest":     event.Digest,
	})
	eventCtx := log.ContextWithLogger(msgCtx, logger)

	logger.Infof("Received Artifact Registry event")

	// Process the event
	if err := s.eventHandler(eventCtx, event); err != nil {
		logger.Errorf("Failed to process event: %v", err)
		msg.Nack()
		return
	}

	logger.Infof("Successfully processed Pub/Sub message")
	msg.Ack()
}

// NeedLeaderElection indicates this runnable should only run on the leader.
func (s *ArtifactRegistrySubscriber) NeedLeaderElection() bool {
	return true
}

// ArtifactRegistryMessage represents the Pub/Sub message payload from Artifact Registry.
// See: https://cloud.google.com/artifact-registry/docs/configure-notifications
type ArtifactRegistryMessage struct {
	Action string `json:"action"`
	Digest string `json:"digest,omitempty"`
	Tag    string `json:"tag,omitempty"`
}

// ParseArtifactRegistryMessage parses a Pub/Sub message from Artifact Registry
// and converts it to a WebhookEvent. This is used by both the push webhook
// and the pull subscriber.
func ParseArtifactRegistryMessage(data []byte) (*argocd.WebhookEvent, error) {
	// Artifact Registry notifications are JSON payloads. For both the Pub/Sub push
	// and pull models, the canonical payload is the decoded message data.
	if len(data) == 0 {
		return nil, ErrEmptyMessageData
	}

	var msg ArtifactRegistryMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal Artifact Registry message: %w", err)
	}

	action := msg.Action
	digest := msg.Digest
	tag := msg.Tag

	// Only process INSERT actions (new images)
	if action != "" && action != "INSERT" {
		return nil, fmt.Errorf("%w: %s", ErrNonInsertAction, action)
	}

	// Extract registry URL, repository and image info from digest
	registryURL, repository, err := parseArtifactRegistryDigest(digest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse digest: %w", err)
	}

	// Extract just the hash from the digest
	digestHash := ""
	if strings.Contains(digest, "@") {
		parts := strings.Split(digest, "@")
		if len(parts) == 2 {
			digestHash = parts[1]
		}
	}

	return &argocd.WebhookEvent{
		RegistryURL: registryURL,
		Repository:  repository,
		Tag:         tag,
		Digest:      digestHash,
	}, nil
}

// parseArtifactRegistryDigest extracts registry URL and repository from an Artifact Registry digest.
// Digest format: LOCATION-docker.pkg.dev/PROJECT/REPO/IMAGE@sha256:HASH
// or: LOCATION-docker.pkg.dev/PROJECT/REPO/IMAGE:TAG
func parseArtifactRegistryDigest(digest string) (registryURL, repository string, err error) {
	if digest == "" {
		return "", "", fmt.Errorf("empty digest")
	}

	// Remove the digest hash or tag suffix to get the image path
	imagePath := digest
	if idx := strings.Index(digest, "@"); idx > 0 {
		imagePath = digest[:idx]
	} else if idx := strings.LastIndex(digest, ":"); idx > 0 {
		imagePath = digest[:idx]
	}

	// Split into registry and repository parts
	parts := strings.SplitN(imagePath, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid image path format: %s", imagePath)
	}

	registryURL = parts[0]
	repository = parts[1]

	// Validate it looks like a GCP container registry
	if !strings.HasSuffix(registryURL, "-docker.pkg.dev") &&
		!strings.HasSuffix(registryURL, ".gcr.io") &&
		registryURL != "gcr.io" {
		return "", "", fmt.Errorf("not a recognized GCP container registry: %s", registryURL)
	}

	return registryURL, repository, nil
}

// isArtifactRegistryIgnorableError returns true for errors that indicate the message should be
// acknowledged and not retried (e.g., non-INSERT actions).
func isArtifactRegistryIgnorableError(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrNonInsertAction) || errors.Is(err, ErrEmptyMessageData)
}
