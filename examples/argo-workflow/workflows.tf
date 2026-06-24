# Workflow dispatched via Argo Workflows. The job_agent config is forwarded to
# the Argo agent on dispatch; `template` carries the inline workflow manifest.
resource "ctrlplane_workflow" "argo" {
  name = "Create Ephemeral Tenant"

  inputs = jsonencode([
    { key = "wandb_version", type = "string", default = "0.80.0" },
    { key = "name", type = "string" },
  ])

  job_agent {
    name     = "argo_workflow"
    ref      = ctrlplane_job_agent.argo.id
    selector = "true"

    config = {
      name          = "create-ephemeral-tenant"
      serverUrl     = "argo-server.argo.svc.cluster.local:2746"
      apiKey        = ""
      webhookSecret = ""
      template      = file("${path.module}/workflow.yaml")
    }
  }
}
