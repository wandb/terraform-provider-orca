resource "ctrlplane_secret_provider" "google" {
  name = "google-secret-manager"
  type = "google_secret_manager"
  config = jsonencode({
    project = var.gcp_project
  })
}

resource "ctrlplane_secret" "test_secret" {
  scope       = "workspace"
  name        = var.gcp_secret_name
  provider_id = ctrlplane_secret_provider.google.id
  path        = [var.gcp_secret_name]
  key         = ""
  version     = "latest"
}
