resource "ctrlplane_secret_provider" "doppler" {
  name = "doppler"
  type = "doppler"
  config = jsonencode({
    serviceToken = var.doppler_service_token
  })
}

resource "ctrlplane_secret" "test_secret" {
  scope       = "workspace"
  name        = lower(var.doppler_secret_name)
  provider_id = ctrlplane_secret_provider.doppler.id
  path        = [var.doppler_project, var.doppler_config]
  key         = var.doppler_secret_name
}
