resource "ctrlplane_job_agent" "argo" {
  name = "argo-workflow-runner"

  argo_workflow {
    server_url     = "argo-server.argo.svc.cluster.local:2746"
    api_key        = var.argo_api_key
    webhook_secret = var.argo_webhook_secret
    template       = "create-ephemeral-tenant"
    name           = "create-ephemeral-tenant"
  }
}
