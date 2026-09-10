data "ctrlplane_deployment" "example" {
  name = "my-deployment"
}

data "ctrlplane_deployment_variable" "image_tag" {
  deployment_id = data.ctrlplane_deployment.example.id
  key           = "IMAGE_TAG"
}

output "deployment_variable_id" {
  value = data.ctrlplane_deployment_variable.image_tag.id
}
