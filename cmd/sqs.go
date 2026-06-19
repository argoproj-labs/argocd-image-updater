package main

import (
	"context"
	"errors"
	"fmt"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/aws"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// SQSPollerRunnable polls an SQS queue for ECR EventBridge events and triggers image updates.
type SQSPollerRunnable struct {
	Reconciler *controller.ImageUpdaterReconciler
	Config     *controller.ImageUpdaterConfig
	consumer   *aws.SQSConsumer
}

// Start long-polls SQS until the context is cancelled.
func (s *SQSPollerRunnable) Start(ctx context.Context) error {
	sqsLogger := common.LogFields(map[string]interface{}{
		"logger": "sqs-poller",
	})
	ctx = log.ContextWithLogger(ctx, sqsLogger)

	cfg := aws.SQSConfig{
		ClientConfig: aws.ClientConfig{
			Region:      s.Config.AWSRegion,
			EndpointURL: s.Config.AWSEndpointURL,
		},
		QueueURL:          s.Config.SQSQueueURL,
		MaxMessages:       s.Config.SQSMaxMessages,
		WaitTimeSeconds:   s.Config.SQSWaitSeconds,
		VisibilityTimeout: s.Config.SQSVisibilityTimeout,
	}

	consumer, err := aws.NewSQSConsumer(ctx, cfg)
	if err != nil {
		return fmt.Errorf("failed to create SQS consumer: %w", err)
	}
	s.consumer = consumer
	sqsLogger.Infof("Starting SQS poller for queue %s", s.Config.SQSQueueURL)

	for {
		select {
		case <-ctx.Done():
			sqsLogger.Infof("Stopping SQS poller")
			return nil
		default:
		}

		events, err := s.consumer.ReceiveEvents(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			sqsLogger.Errorf("Failed to receive SQS messages: %v", err)
			continue
		}

		for _, received := range events {
			if err := s.processEvent(ctx, received); err != nil {
				sqsLogger.Errorf("Failed to process SQS message %s: %v", received.MessageID, err)
				continue
			}
			if err := s.consumer.DeleteMessage(ctx, received.ReceiptHandle); err != nil {
				sqsLogger.Errorf("Failed to delete SQS message %s: %v", received.MessageID, err)
			}
		}
	}
}

func (s *SQSPollerRunnable) processEvent(ctx context.Context, received aws.ReceivedEvent) error {
	logCtx := log.LoggerFromContext(ctx)
	pushEvent := received.Event
	logCtx.Infof("Processing ECR push event for %s/%s:%s", pushEvent.RegistryURL, pushEvent.Repository, pushEvent.Tag)

	event := &argocd.WebhookEvent{
		RegistryURL: pushEvent.RegistryURL,
		Repository:  pushEvent.Repository,
		Tag:         pushEvent.Tag,
		Digest:      pushEvent.Digest,
	}

	processingCtx := log.ContextWithLogger(context.Background(), logCtx)
	imageList := &api.ImageUpdaterList{}
	if err := s.Reconciler.List(processingCtx, imageList); err != nil {
		return fmt.Errorf("failed to list ImageUpdater CRs: %w", err)
	}

	if err := s.Reconciler.ProcessImageUpdaterCRs(processingCtx, imageList.Items, false, event); err != nil {
		return fmt.Errorf("failed to process ImageUpdater CRs: %w", err)
	}
	return nil
}

// NeedLeaderElection ensures only the leader polls SQS.
func (s *SQSPollerRunnable) NeedLeaderElection() bool {
	return true
}
