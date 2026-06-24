terraform {
  required_providers {
    ctrlplane = {
      source  = "wandb/orca"
      version = ">= 1.0.2"
    }
  }
}

provider "ctrlplane" {
  workspace = var.workspace
  url       = var.url
  api_key   = var.api_key
}
