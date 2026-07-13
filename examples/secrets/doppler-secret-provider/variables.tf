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

variable "doppler_service_token" {
  type        = string
  description = "Doppler service token (dp.st.*) scoped to the project/config"
  sensitive   = true
}

variable "doppler_project" {
  type        = string
  description = "The Doppler project"
  default     = "backend"
}

variable "doppler_config" {
  type        = string
  description = "The Doppler config within the project"
  default     = "dev"
}

variable "doppler_secret_name" {
  type        = string
  description = "The Doppler secret name within the config"
  default     = "MY_SECRET"
}
