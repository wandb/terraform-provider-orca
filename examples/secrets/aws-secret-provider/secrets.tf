resource "ctrlplane_secret_provider" "aws" {
  name = "aws-secrets-manager"
  type = "aws_secrets_manager"
  config = jsonencode({
    region = var.aws_region
  })
}

resource "ctrlplane_secret" "test_secret" {
  scope       = "workspace"
  name        = "aws-${var.aws_secret_name}"
  provider_id = ctrlplane_secret_provider.aws.id
  path        = [var.aws_secret_name]
  key         = ""
}
