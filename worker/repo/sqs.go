package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/monitoring/telemetry"
	"github.com/ashwinsekaran/simple_platform_app/worker/config"
	"github.com/ashwinsekaran/simple_platform_app/worker/ent"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamoTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type SQSRepository struct {
	sqsClient *sqs.Client
	ddbClient *dynamodb.Client
	queueURL  string
	dlqURL    string
	tableName string
}

var dlqMetrics struct {
	once         sync.Once
	messageCount metric.Int64ObservableGauge
	replays      metric.Int64Counter
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
		sqsClient: sqs.NewFromConfig(awsCfg),
		ddbClient: dynamodb.NewFromConfig(awsCfg),
		queueURL:  cfg.SQSQueueURL,
		dlqURL:    cfg.DLQQueueURL,
		tableName: cfg.DynamoTableName,
	}, nil
}

func (r *SQSRepository) ReceiveDLQEvents(ctx context.Context, maxMessages int32) ([]ReceivedEvent, error) {
	if r.dlqURL == "" {
		return nil, fmt.Errorf("dlq queue url is not configured")
	}

	return r.receiveFromQueue(ctx, r.dlqURL, maxMessages)
}

func (r *SQSRepository) ReplayDLQEvent(ctx context.Context, event ReceivedEvent) error {
	if r.dlqURL == "" {
		return fmt.Errorf("dlq queue url is not configured")
	}

	if err := r.sendEvent(ctx, r.queueURL, event.Event, event.Attributes); err != nil {
		return err
	}

	_, err := r.sqsClient.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(r.dlqURL),
		ReceiptHandle: aws.String(event.ReceiptHandle),
	})
	if err != nil {
		return fmt.Errorf("delete dlq message: %w", err)
	}

	dlqInstruments().replays.Add(ctx, 1)
	return nil
}

func (r *SQSRepository) StartDLQMonitoring(ctx context.Context, interval time.Duration) error {
	if r.dlqURL == "" {
		return fmt.Errorf("dlq queue url is not configured")
	}

	meter := otel.Meter("simple_platform_setup/dlq")
	instruments := dlqInstruments()
	_, err := meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		count, err := r.DLQMessageCount(ctx)
		if err != nil {
			telemetry.Log(ctx, "dlq count fetch failed: %v", err)
			return nil
		}

		observer.ObserveInt64(instruments.messageCount, count)
		return nil
	}, instruments.messageCount)
	if err != nil {
		return fmt.Errorf("register dlq callback: %w", err)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *SQSRepository) DLQMessageCount(ctx context.Context) (int64, error) {
	if r.dlqURL == "" {
		return 0, fmt.Errorf("dlq queue url is not configured")
	}

	output, err := r.sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl:       aws.String(r.dlqURL),
		AttributeNames: []sqsTypes.QueueAttributeName{sqsTypes.QueueAttributeNameApproximateNumberOfMessages},
	})
	if err != nil {
		return 0, fmt.Errorf("get dlq attributes: %w", err)
	}

	value := output.Attributes[string(sqsTypes.QueueAttributeNameApproximateNumberOfMessages)]
	if value == "" {
		return 0, nil
	}

	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse dlq message count: %w", err)
	}

	return count, nil
}

func (r *SQSRepository) UpdateProcessingResult(ctx context.Context, id, status, result string) error {
	_, err := r.ddbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]dynamoTypes.AttributeValue{
			"id": &dynamoTypes.AttributeValueMemberS{Value: id},
		},
		UpdateExpression: aws.String("SET processing_status = :status, processing_result = :result"),
		ExpressionAttributeValues: map[string]dynamoTypes.AttributeValue{
			":status": &dynamoTypes.AttributeValueMemberS{Value: status},
			":result": &dynamoTypes.AttributeValueMemberS{Value: result},
		},
	})
	if err != nil {
		return fmt.Errorf("update processing result: %w", err)
	}

	return nil
}

func (r *SQSRepository) receiveFromQueue(ctx context.Context, queueURL string, maxMessages int32) ([]ReceivedEvent, error) {
	output, err := r.sqsClient.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:              aws.String(queueURL),
		MaxNumberOfMessages:   maxMessages,
		WaitTimeSeconds:       20,
		VisibilityTimeout:     30,
		MessageAttributeNames: []string{"All"},
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

		attributes := make(map[string]string, len(message.MessageAttributes))
		for key, attribute := range message.MessageAttributes {
			if attribute.StringValue != nil {
				attributes[key] = aws.ToString(attribute.StringValue)
			}
		}

		var ingestedAt time.Time
		if value, ok := attributes["ingested_at_unix_nano"]; ok {
			nanos, err := strconv.ParseInt(value, 10, 64)
			if err == nil {
				ingestedAt = time.Unix(0, nanos).UTC()
			}
		}

		events = append(events, ReceivedEvent{
			Event:         event,
			Attributes:    attributes,
			IngestedAt:    ingestedAt,
			ReceiptHandle: aws.ToString(message.ReceiptHandle),
		})
	}

	return events, nil
}

func (r *SQSRepository) sendEvent(ctx context.Context, queueURL string, event ent.Event, attributes map[string]string) error {
	messageAttributes := make(map[string]sqsTypes.MessageAttributeValue, len(attributes))
	for key, value := range attributes {
		messageAttributes[key] = sqsTypes.MessageAttributeValue{
			DataType:    aws.String("String"),
			StringValue: aws.String(value),
		}
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	_, err = r.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:          aws.String(queueURL),
		MessageBody:       aws.String(string(body)),
		MessageAttributes: messageAttributes,
	})
	if err != nil {
		return fmt.Errorf("send message: %w", err)
	}

	return nil
}

func dlqInstruments() *struct {
	once         sync.Once
	messageCount metric.Int64ObservableGauge
	replays      metric.Int64Counter
} {
	dlqMetrics.once.Do(func() {
		meter := otel.Meter("simple_platform_setup/dlq")
		dlqMetrics.messageCount, _ = meter.Int64ObservableGauge("worker_dlq_message_count")
		dlqMetrics.replays, _ = meter.Int64Counter("worker_dlq_replay_total")
	})

	return &dlqMetrics
}
