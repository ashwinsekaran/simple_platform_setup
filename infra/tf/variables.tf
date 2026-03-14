variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "aws_endpoint_url" {
  type    = string
  default = "http://localhost:4566"
}

variable "aws_access_key_id" {
  type    = string
  default = "test"
}

variable "aws_secret_access_key" {
  type    = string
  default = "test"
}

variable "project_name" {
  type    = string
  default = "simple-platform"
}

variable "queue_name" {
  type    = string
  default = "ingest-events"
}

variable "lambda_name" {
  type    = string
  default = "ingest-events-consumer"
}
