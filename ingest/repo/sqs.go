package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/ashwinsekaran/simple_platform_app/ingest/config"
	"github.com/ashwinsekaran/simple_platform_app/ingest/ent"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamoTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqsTypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type SQSRepository struct {
	sqsClient *sqs.Client
	ddbClient *dynamodb.Client
	queueURL  string
	tableName string
}

var repoMetrics struct {
	once               sync.Once
	dynamoDurationMS   metric.Float64Histogram
	dynamoErrors       metric.Int64Counter
	sqsPublishDuration metric.Float64Histogram
	sqsPublishErrors   metric.Int64Counter
}

type storedEvent struct {
	ID               string `dynamodbav:"id"`
	Type             string `dynamodbav:"type"`
	Payload          string `dynamodbav:"payload"`
	Published        bool   `dynamodbav:"published"`
	ProcessingStatus string `dynamodbav:"processing_status"`
	ProcessingResult string `dynamodbav:"processing_result"`
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
		tableName: cfg.DynamoTableName,
	}, nil
}

func (r *SQSRepository) SaveEvent(ctx context.Context, event ent.Event) (bool, error) {
	item := storedEvent{
		ID:               event.ID,
		Type:             event.Type,
		Payload:          string(event.Payload),
		Published:        false,
		ProcessingStatus: "queued",
		ProcessingResult: "",
	}

	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return false, fmt.Errorf("marshal event item: %w", err)
	}

	start := time.Now()
	_, err = r.ddbClient.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           aws.String(r.tableName),
		Item:                av,
		ConditionExpression: aws.String("attribute_not_exists(id)"),
	})
	repoInstruments().dynamoDurationMS.Record(ctx, float64(time.Since(start).Milliseconds()), metric.WithAttributes(attribute.String("operation", "put_item")))
	if err != nil {
		var conditionalCheck *dynamoTypes.ConditionalCheckFailedException
		if !errors.As(err, &conditionalCheck) {
			repoInstruments().dynamoErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "put_item")))
			return false, fmt.Errorf("store event: %w", err)
		}

		existing, getErr := r.getStoredEvent(ctx, event.ID)
		if getErr != nil {
			return false, getErr
		}

		if existing.Type != event.Type || existing.Payload != string(event.Payload) {
			return false, ErrEventConflict
		}

		if existing.Published {
			return false, nil
		}
	} else {
		if err := r.publishEvent(ctx, event); err != nil {
			return false, err
		}

		if err := r.markPublished(ctx, event.ID); err != nil {
			return false, err
		}

		return true, nil
	}

	if err := r.publishEvent(ctx, event); err != nil {
		return false, err
	}

	if err := r.markPublished(ctx, event.ID); err != nil {
		return false, err
	}

	return false, nil
}

func (r *SQSRepository) GetEvent(ctx context.Context, id string) (ent.EventRecord, error) {
	item, err := r.getStoredEvent(ctx, id)
	if err != nil {
		return ent.EventRecord{}, err
	}

	return ent.EventRecord{
		ID:               item.ID,
		Type:             item.Type,
		Payload:          json.RawMessage(item.Payload),
		ProcessingStatus: item.ProcessingStatus,
		ProcessingResult: item.ProcessingResult,
	}, nil
}

func (r *SQSRepository) Ready(ctx context.Context) error {
	start := time.Now()
	if _, err := r.sqsClient.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(r.queueURL),
	}); err != nil {
		repoInstruments().sqsPublishErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "queue_readiness")))
		return fmt.Errorf("check queue readiness: %w", err)
	} else {
		repoInstruments().sqsPublishDuration.Record(ctx, float64(time.Since(start).Milliseconds()), metric.WithAttributes(attribute.String("operation", "queue_readiness")))
	}

	start = time.Now()
	if _, err := r.ddbClient.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(r.tableName),
	}); err != nil {
		repoInstruments().dynamoErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "describe_table")))
		return fmt.Errorf("check dynamodb readiness: %w", err)
	} else {
		repoInstruments().dynamoDurationMS.Record(ctx, float64(time.Since(start).Milliseconds()), metric.WithAttributes(attribute.String("operation", "describe_table")))
	}

	return nil
}

func (r *SQSRepository) getStoredEvent(ctx context.Context, id string) (storedEvent, error) {
	start := time.Now()
	output, err := r.ddbClient.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]dynamoTypes.AttributeValue{
			"id": &dynamoTypes.AttributeValueMemberS{Value: id},
		},
		ConsistentRead: aws.Bool(true),
	})
	repoInstruments().dynamoDurationMS.Record(ctx, float64(time.Since(start).Milliseconds()), metric.WithAttributes(attribute.String("operation", "get_item")))
	if err != nil {
		repoInstruments().dynamoErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "get_item")))
		return storedEvent{}, fmt.Errorf("get event: %w", err)
	}

	if len(output.Item) == 0 {
		return storedEvent{}, ErrEventNotFound
	}

	var item storedEvent
	if err := attributevalue.UnmarshalMap(output.Item, &item); err != nil {
		return storedEvent{}, fmt.Errorf("unmarshal event item: %w", err)
	}

	return item, nil
}

func (r *SQSRepository) markPublished(ctx context.Context, id string) error {
	start := time.Now()
	_, err := r.ddbClient.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(r.tableName),
		Key: map[string]dynamoTypes.AttributeValue{
			"id": &dynamoTypes.AttributeValueMemberS{Value: id},
		},
		UpdateExpression: aws.String("SET published = :published"),
		ExpressionAttributeValues: map[string]dynamoTypes.AttributeValue{
			":published": &dynamoTypes.AttributeValueMemberBOOL{Value: true},
		},
	})
	repoInstruments().dynamoDurationMS.Record(ctx, float64(time.Since(start).Milliseconds()), metric.WithAttributes(attribute.String("operation", "update_item")))
	if err != nil {
		repoInstruments().dynamoErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "update_item")))
		return fmt.Errorf("mark event published: %w", err)
	}

	return nil
}

func (r *SQSRepository) publishEvent(ctx context.Context, event ent.Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	attributes := messageAttributes(event)
	attributes["ingested_at_unix_nano"] = sqsTypes.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(strconv.FormatInt(time.Now().UTC().UnixNano(), 10)),
	}
	otel.GetTextMapPropagator().Inject(ctx, messageAttributeCarrier(attributes))

	start := time.Now()
	_, err = r.sqsClient.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:          aws.String(r.queueURL),
		MessageBody:       aws.String(string(body)),
		MessageAttributes: attributes,
	})
	repoInstruments().sqsPublishDuration.Record(ctx, float64(time.Since(start).Milliseconds()), metric.WithAttributes(attribute.String("operation", "send_message")))
	if err != nil {
		repoInstruments().sqsPublishErrors.Add(ctx, 1, metric.WithAttributes(attribute.String("operation", "send_message")))
		return fmt.Errorf("send sqs message: %w", err)
	}

	return nil
}

func repoInstruments() *struct {
	once               sync.Once
	dynamoDurationMS   metric.Float64Histogram
	dynamoErrors       metric.Int64Counter
	sqsPublishDuration metric.Float64Histogram
	sqsPublishErrors   metric.Int64Counter
} {
	repoMetrics.once.Do(func() {
		meter := otel.Meter("simple_platform_setup/ingest_repo")
		repoMetrics.dynamoDurationMS, _ = meter.Float64Histogram("ingest_dynamodb_operation_duration_ms")
		repoMetrics.dynamoErrors, _ = meter.Int64Counter("ingest_dynamodb_error_total")
		repoMetrics.sqsPublishDuration, _ = meter.Float64Histogram("ingest_sqs_operation_duration_ms")
		repoMetrics.sqsPublishErrors, _ = meter.Int64Counter("ingest_sqs_error_total")
	})

	return &repoMetrics
}

func messageAttributes(event ent.Event) map[string]sqsTypes.MessageAttributeValue {
	return map[string]sqsTypes.MessageAttributeValue{
		"event_type": {
			DataType:    aws.String("String"),
			StringValue: aws.String(event.Type),
		},
		"event_id": {
			DataType:    aws.String("String"),
			StringValue: aws.String(event.ID),
		},
	}
}

type messageAttributeCarrier map[string]sqsTypes.MessageAttributeValue

func (c messageAttributeCarrier) Get(key string) string {
	attribute, ok := c[key]
	if !ok || attribute.StringValue == nil {
		return ""
	}

	return aws.ToString(attribute.StringValue)
}

func (c messageAttributeCarrier) Set(key, value string) {
	c[key] = sqsTypes.MessageAttributeValue{
		DataType:    aws.String("String"),
		StringValue: aws.String(value),
	}
}

func (c messageAttributeCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for key := range c {
		keys = append(keys, key)
	}

	return keys
}
