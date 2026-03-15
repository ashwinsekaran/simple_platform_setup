locals {
  common_tags = {
    project = var.project_name
    managed = "terraform"
  }
}

resource "aws_sqs_queue" "ingest_events" {
  name                       = var.queue_name
  visibility_timeout_seconds = 30
  message_retention_seconds  = 345600
  redrive_policy = jsonencode({
    deadLetterTargetArn = aws_sqs_queue.ingest_events_dlq.arn
    maxReceiveCount     = var.max_receive_count
  })
  tags = local.common_tags
}

resource "aws_sqs_queue" "ingest_events_dlq" {
  name                      = var.dlq_queue_name
  message_retention_seconds = 1209600
  tags                      = local.common_tags
}

resource "aws_dynamodb_table" "ingest_events" {
  name         = var.table_name
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "id"

  attribute {
    name = "id"
    type = "S"
  }

  tags = local.common_tags
}

resource "aws_iam_role" "worker_lambda" {
  name = "${var.lambda_function_name}-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy" "worker_lambda" {
  name = "${var.lambda_function_name}-policy"
  role = aws_iam_role.worker_lambda.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "dynamodb:UpdateItem",
          "dynamodb:GetItem"
        ]
        Resource = aws_dynamodb_table.ingest_events.arn
      },
      {
        Effect = "Allow"
        Action = [
          "sqs:ReceiveMessage",
          "sqs:DeleteMessage",
          "sqs:GetQueueAttributes"
        ]
        Resource = aws_sqs_queue.ingest_events.arn
      }
    ]
  })
}

resource "aws_lambda_function" "worker" {
  function_name    = var.lambda_function_name
  role             = aws_iam_role.worker_lambda.arn
  filename         = "${path.module}/build/worker-lambda.zip"
  source_code_hash = filebase64sha256("${path.module}/build/worker-lambda.zip")
  runtime          = "provided.al2023"
  handler          = "bootstrap"
  timeout          = 30

  environment {
    variables = {
      AWS_REGION                  = var.aws_region
      AWS_ENDPOINT_URL            = "http://localstack:4566"
      AWS_ACCESS_KEY_ID           = var.aws_access_key_id
      AWS_SECRET_ACCESS_KEY       = var.aws_secret_access_key
      OTEL_EXPORTER_OTLP_ENDPOINT = "otel-collector:4317"
      INGEST_DYNAMODB_TABLE       = aws_dynamodb_table.ingest_events.name
    }
  }

  tags = local.common_tags
}

resource "aws_lambda_event_source_mapping" "worker_sqs" {
  event_source_arn        = aws_sqs_queue.ingest_events.arn
  function_name           = aws_lambda_function.worker.arn
  batch_size              = 10
  function_response_types = ["ReportBatchItemFailures"]
}
