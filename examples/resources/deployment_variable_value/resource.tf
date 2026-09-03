# Literal value example
resource "ctrlplane_deployment_variable_value" "literal_example" {
  variable_id = ctrlplane_deployment_variable.example.id
  priority    = 1

  resource_selector = <<EOT
    resource.metadata["environment"] == "production"
  EOT

  literal_value = "my-production-value"
}

# Reference value example
resource "ctrlplane_deployment_variable_value" "reference_example" {
  variable_id = ctrlplane_deployment_variable.example.id
  priority    = 2

  resource_selector = <<EOT
    resource.kind == "kubernetes/cluster"
  EOT

  reference_value {
    reference = "key"
    path      = ["metadata", "cluster_name"]
  }
}
