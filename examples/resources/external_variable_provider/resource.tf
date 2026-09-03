variable "deployment_id" {
  type        = string
  description = "The Ctrlplane deployment that owns the Statsig-backed variable"
}

variable "ctrlplane_url" {
  type        = string
  description = "The local workspace engine URL"
}

variable "statsig_endpoint" {
  type        = string
  description = "The Statsig API endpoint"
}

variable "statsig_console_url" {
  type        = string
  description = "The Statsig project console URL"
}

# The workspace engine process must have STATSIG_API_KEY in its environment.
resource "ctrlplane_secret_provider" "local_env" {
  name = "local-env"
  type = "env"

  config_wo = jsonencode({
    allowedKeys = ["STATSIG_API_KEY"]
  })
}

resource "ctrlplane_secret" "statsig_server_key" {
  scope       = "workspace"
  name        = "statsig-server-key"
  provider_id = ctrlplane_secret_provider.local_env.id
  path        = []
  key         = "STATSIG_API_KEY"
}

resource "ctrlplane_external_variable_provider" "statsig" {
  name = "statsig-production"

  statsig {
    endpoint    = var.statsig_endpoint
    token       = "{{ secret \"statsig-server-key\" }}"
    console_url = var.statsig_console_url
  }

  depends_on = [ctrlplane_secret.statsig_server_key]
}

resource "ctrlplane_deployment_variable" "enable_new_checkout" {
  deployment_id = var.deployment_id
  key           = "ENABLE_NEW_CHECKOUT"
  description   = "Resolved through Statsig for each resource"
}

resource "ctrlplane_deployment_variable_value" "enable_new_checkout" {
  variable_id = ctrlplane_deployment_variable.enable_new_checkout.id
  priority    = 0

  statsig {
    provider_id = ctrlplane_external_variable_provider.statsig.id
    gate_key    = "enable_new_checkout"

    user_template = jsonencode({
      userID = "{{ .resource.id }}"
      custom = {
        customerNamespace = "{{ .resource.name }}"
        region            = "{{ .resource.metadata.region }}"
      }
      customIDs = {
        deploymentID = "{{ .resource.identifier }}"
        stableID     = "{{ .resource.id }}"
      }
      statsigEnvironment = {
        tier = "production"
      }
    })
  }
}
