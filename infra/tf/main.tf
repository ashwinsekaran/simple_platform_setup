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
