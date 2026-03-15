package config

import "os"

type Config struct {
	HTTPAddr         string
	AWSRegion        string
	AWSEndpoint      string
	OTELEndpoint     string
	AWSAccessKeyID   string
	AWSSecretKey     string
	DynamoTableName  string
	SQSQueueURL      string
	ShutdownTimeoutS int
}

func Load() Config {
	return Config{
		HTTPAddr:         getEnv("INGEST_HTTP_ADDR", ":8080"),
		AWSRegion:        getEnv("AWS_REGION", "us-east-1"),
		AWSEndpoint:      getEnv("AWS_ENDPOINT_URL", "http://localhost:4566"),
		OTELEndpoint:     getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		AWSAccessKeyID:   getEnv("AWS_ACCESS_KEY_ID", "test"),
		AWSSecretKey:     getEnv("AWS_SECRET_ACCESS_KEY", "test"),
		DynamoTableName:  getEnv("INGEST_DYNAMODB_TABLE", "ingest-events"),
		SQSQueueURL:      getEnv("INGEST_SQS_QUEUE_URL", "http://localhost:4566/000000000000/ingest-events"),
		ShutdownTimeoutS: 5,
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
