variable "doppler_config" {
  type        = string
  description = "Doppler secret-provider configuration serialized as JSON"
  sensitive   = true
  ephemeral   = true
}

# Prefer a read-only Service Token scoped to one Doppler config. Use a
# Service Account Token only when broader project access is required.
resource "ctrlplane_secret_provider" "doppler" {
  name              = "doppler-production"
  type              = "doppler"
  config_wo         = var.doppler_config
  config_wo_version = 1
}
