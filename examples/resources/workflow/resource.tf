# Workflow dispatched via GitHub Actions.
resource "ctrlplane_workflow" "github" {
  name = "deploy-service"

  inputs = jsonencode([
    { key = "wandb_version", type = "string", default = "0.80.0" },
    { key = "name", type = "string" },
  ])

  job_agent {
    name     = "github_actions"
    ref      = ctrlplane_job_agent.github.id
    selector = "true"

    config = {
      installationId = "12345678"
      owner          = "my-org"
      repo           = "deployments"
      workflowId     = "deploy.yaml"
      ref            = "main"
    }
  }
}

# Workflow dispatched via Argo Workflows. The job_agent config is forwarded to
# the Argo agent on dispatch; `template` carries the inline workflow manifest.
resource "ctrlplane_workflow" "argo" {
  name = "create-ephemeral-tenant"

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
