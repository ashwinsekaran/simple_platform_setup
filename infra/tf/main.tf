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
