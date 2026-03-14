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
  tags                       = local.common_tags
}

resource "aws_iam_role" "lambda_exec" {
  name = "${var.lambda_name}-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })

  tags = local.common_tags
}

resource "aws_iam_role_policy" "lambda_sqs" {
  name = "${var.lambda_name}-policy"
  role = aws_iam_role.lambda_exec.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "*"
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

data "archive_file" "lambda_zip" {
  type        = "zip"
  source_file = "${path.module}/lambda/index.js"
  output_path = "${path.module}/build/ingest-events-consumer.zip"
}

resource "aws_lambda_function" "ingest_events_consumer" {
  function_name    = var.lambda_name
  role             = aws_iam_role.lambda_exec.arn
  runtime          = "nodejs20.x"
  handler          = "index.handler"
  filename         = data.archive_file.lambda_zip.output_path
  source_code_hash = data.archive_file.lambda_zip.output_base64sha256
  timeout          = 30
  tags             = local.common_tags
}

resource "aws_lambda_event_source_mapping" "ingest_events_sqs" {
  event_source_arn = aws_sqs_queue.ingest_events.arn
  function_name    = aws_lambda_function.ingest_events_consumer.arn
  batch_size       = 10
  enabled          = true
}
