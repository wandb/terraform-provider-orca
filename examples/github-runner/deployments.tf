resource "ctrlplane_deployment" "this" {
  name               = "github-runner-deployment"
  resource_selector  = "resource.name == 'github-runner-test'"
  job_agent_selector = "jobAgent.id == \"${ctrlplane_job_agent.this.id}\""

  github {
    repo        = "orca-gh-get-job-inputs-test"
    workflow_id = 301102333
  }
}

resource "ctrlplane_deployment_system_link" "this" {
  deployment_id = ctrlplane_deployment.this.id
  system_id     = ctrlplane_system.this.id
}
