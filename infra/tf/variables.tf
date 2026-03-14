variable "aws_region" {
  type    = string
  default = "us-east-1"
}

variable "aws_endpoint_url" {
  type    = string
  default = "http://localhost:4566"
}

variable "aws_access_key_id" {
  type      = string
  sensitive = true
}

variable "aws_secret_access_key" {
  type      = string
  sensitive = true
}

variable "project_name" {
  type    = string
  default = "simple-platform"
}

variable "queue_name" {
  type    = string
  default = "ingest-events"
}
