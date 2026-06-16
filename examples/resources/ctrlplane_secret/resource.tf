resource "ctrlplane_secret_provider" "aws" {
  name = "aws-prod"
  type = "aws-secrets-manager"
  config = jsonencode({
    region = "us-east-1"
  })
}

resource "ctrlplane_secret" "db_password" {
  scope       = "workspace"
  name        = "db-password"
  provider_id = ctrlplane_secret_provider.aws.id
  path        = ["secret", "data", "db"]
  key         = "password"
  version     = "v1"
}
