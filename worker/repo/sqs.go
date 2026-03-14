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

func (r *SQSRepository) ReceiveEvents(ctx context.Context, maxMessages int32) ([]ReceivedEvent, error) {
	output, err := r.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(r.queueURL),
		MaxNumberOfMessages: maxMessages,
		WaitTimeSeconds:     20,
		VisibilityTimeout:   30,
	})
	if err != nil {
		return nil, fmt.Errorf("receive messages: %w", err)
	}

	events := make([]ReceivedEvent, 0, len(output.Messages))
	for _, message := range output.Messages {
		var event ent.Event
		if err := json.Unmarshal([]byte(aws.ToString(message.Body)), &event); err != nil {
			return nil, fmt.Errorf("unmarshal message: %w", err)
		}

		events = append(events, ReceivedEvent{
			Event:         event,
			ReceiptHandle: aws.ToString(message.ReceiptHandle),
		})
	}

	return events, nil
}

func (r *SQSRepository) DeleteEvent(ctx context.Context, receiptHandle string) error {
	_, err := r.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(r.queueURL),
		ReceiptHandle: aws.String(receiptHandle),
	})
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}

	return nil
}
