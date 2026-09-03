terraform {
  required_providers {
    ctrlplane = {
      source = "wandb/orca"
    }
  }
}

provider "ctrlplane" {
  url = var.ctrlplane_url
}
