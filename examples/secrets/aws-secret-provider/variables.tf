variable "workspace" {
  type        = string
  description = "The workspace to use"
}

variable "url" {
  type        = string
  description = "The URL of the Ctrlplane API"
}

variable "api_key" {
  type        = string
  description = "The API key for the Ctrlplane API"
  sensitive   = true
}

variable "aws_region" {
  type        = string
  description = "The AWS region that holds the Secrets Manager secrets"
  default     = "us-east-1"
}

variable "aws_secret_name" {
  type        = string
  description = "The Secrets Manager secret name or ARN to reference"
  default     = "orca-test-secret"
}
