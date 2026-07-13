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

variable "env_var_name" {
  type        = string
  description = "The workspace-engine environment variable to expose as a secret"
  default     = "ORCA_EXAMPLE_SECRET"
}
