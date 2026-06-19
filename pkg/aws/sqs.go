package aws

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
)

// SQSConfig holds configuration for the SQS event consumer.
type SQSConfig struct {
	ClientConfig
	QueueURL           string
	MaxMessages        int32
	WaitTimeSeconds    int32
	VisibilityTimeout  int32
}

// SQSConsumer polls an SQS queue for EventBridge ECR push events.
type SQSConsumer struct {
	client *sqs.Client
	config SQSConfig
}

// NewSQSConsumer creates an SQS consumer for the given configuration.
func NewSQSConsumer(ctx context.Context, cfg SQSConfig) (*SQSConsumer, error) {
	if cfg.QueueURL == "" {
		return nil, fmt.Errorf("sqs queue url is required")
	}
	awsCfg, err := loadAWSConfig(ctx, cfg.ClientConfig)
	if err != nil {
		return nil, err
	}
	return &SQSConsumer{
		client: sqs.NewFromConfig(awsCfg),
		config: cfg,
	}, nil
}

// ReceiveEvents long-polls SQS and returns parsed webhook events with receipt handles.
func (c *SQSConsumer) ReceiveEvents(ctx context.Context) ([]ReceivedEvent, error) {
	maxMessages := c.config.MaxMessages
	if maxMessages <= 0 {
		maxMessages = 10
	}
	waitSeconds := c.config.WaitTimeSeconds
	if waitSeconds <= 0 {
		waitSeconds = 20
	}

	input := &sqs.ReceiveMessageInput{
		QueueUrl:            &c.config.QueueURL,
		MaxNumberOfMessages: maxMessages,
		WaitTimeSeconds:     waitSeconds,
	}
	if c.config.VisibilityTimeout > 0 {
		input.VisibilityTimeout = c.config.VisibilityTimeout
	}

	out, err := c.client.ReceiveMessage(ctx, input)
	if err != nil {
		return nil, err
	}

	events := make([]ReceivedEvent, 0, len(out.Messages))
	for _, msg := range out.Messages {
		if msg.Body == nil || msg.ReceiptHandle == nil {
			continue
		}
		event, err := ParseECREventBridgeMessage([]byte(*msg.Body))
		if err != nil {
			if errors.Is(err, ErrSkippedEvent) {
				// Delete unsupported events so they do not block the queue.
				_ = c.DeleteMessage(ctx, *msg.ReceiptHandle)
				continue
			}
			return nil, fmt.Errorf("failed to parse sqs message %s: %w", derefString(msg.MessageId), err)
		}
		events = append(events, ReceivedEvent{
			Event:         event,
			ReceiptHandle: *msg.ReceiptHandle,
			MessageID:     derefString(msg.MessageId),
		})
	}
	return events, nil
}

// DeleteMessage removes a processed message from the queue.
func (c *SQSConsumer) DeleteMessage(ctx context.Context, receiptHandle string) error {
	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      &c.config.QueueURL,
		ReceiptHandle: &receiptHandle,
	})
	return err
}

// ReceivedEvent is a parsed ECR push event with its SQS receipt handle.
type ReceivedEvent struct {
	Event         *ImagePushEvent
	ReceiptHandle string
	MessageID     string
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
