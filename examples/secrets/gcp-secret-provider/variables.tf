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

variable "gcp_project" {
  type        = string
  description = "The GCP project that holds the Secret Manager secrets"
}

variable "gcp_secret_name" {
  type        = string
  description = "The Secret Manager secret id to reference"
  default     = "test-secret"
}
