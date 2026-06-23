resource "ctrlplane_secret_provider" "aws" {
  name = "aws-prod"
  type = "aws-secrets-manager"
  config = jsonencode({
    region = "us-east-1"
  })
}
