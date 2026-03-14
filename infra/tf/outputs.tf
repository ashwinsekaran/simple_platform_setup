output "ingest_queue_arn" {
  value = aws_sqs_queue.ingest_events.arn
}

output "ingest_queue_url" {
  value = aws_sqs_queue.ingest_events.url
}

output "lambda_function_name" {
  value = aws_lambda_function.ingest_events_consumer.function_name
}
