package repo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ashwinsekaran/simple_platform_app/ingest/config"
	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

type SQSRepository struct {
	client   *sqs.Client
	queueURL string
}

func NewSQSRepository(ctx context.Context, cfg config.Config) (*SQSRepository, error) {
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.AWSRegion),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(cfg.AWSAccessKeyID, cfg.AWSSecretKey, "")),
	}

	if cfg.AWSEndpoint != "" {
		resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:               cfg.AWSEndpoint,
				HostnameImmutable: true,
			}, nil
		})
		loadOptions = append(loadOptions, awsconfig.WithEndpointResolverWithOptions(resolver))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	return &SQSRepository{
		client:   sqs.NewFromConfig(awsCfg),
		queueURL: cfg.SQSQueueURL,
	}, nil
}

func (r *SQSRepository) PublishEvent(ctx context.Context, event ent.Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	_, err = r.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(r.queueURL),
		MessageBody: aws.String(string(body)),
		MessageAttributes: map[string]sqsTypes.MessageAttributeValue{
			"event_type": {
				DataType:    aws.String("String"),
				StringValue: aws.String(event.Type),
			},
			"event_id": {
				DataType:    aws.String("String"),
				StringValue: aws.String(event.ID),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("send sqs message: %w", err)
	}

	return nil
}
