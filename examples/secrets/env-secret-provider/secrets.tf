resource "ctrlplane_secret_provider" "env" {
  name = "engine-env"
  type = "env"
  config = jsonencode({
    allowedKeys = [var.env_var_name]
  })
}

resource "ctrlplane_secret" "test_secret" {
  scope       = "workspace"
  name        = lower(var.env_var_name)
  provider_id = ctrlplane_secret_provider.env.id
  path        = [var.env_var_name]
  key         = var.env_var_name
}
