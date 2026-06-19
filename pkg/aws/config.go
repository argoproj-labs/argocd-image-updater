package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// ClientConfig holds AWS client configuration shared by SQS and ECR clients.
type ClientConfig struct {
	Region      string
	EndpointURL string
}

func (c ClientConfig) loadOptions(ctx context.Context) ([]func(*awsconfig.LoadOptions) error, error) {
	if c.Region == "" {
		return nil, ErrMissingRegion
	}

	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(c.Region),
	}
	if c.EndpointURL != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, _ ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               c.EndpointURL,
				HostnameImmutable: true,
			}, nil
		})
		opts = append(opts,
			awsconfig.WithEndpointResolverWithOptions(customResolver),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		)
	}
	return opts, nil
}

func loadAWSConfig(ctx context.Context, cfg ClientConfig) (aws.Config, error) {
	opts, err := cfg.loadOptions(ctx)
	if err != nil {
		return aws.Config{}, err
	}
	return awsconfig.LoadDefaultConfig(ctx, opts...)
}
