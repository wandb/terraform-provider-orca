# Workflow dispatched via a GitHub Actions workflow file. The job_agent config
# selects which workflow (`workflowId`) on which ref runs when this dispatches.
resource "ctrlplane_workflow" "github" {
  name = "Test GitHub Actions Workflow"

  inputs = jsonencode([
    { key = "wandb_version", type = "string", default = "0.80.0" },
    { key = "name", type = "string" },
  ])

  job_agent {
    name     = "github_actions"
    ref      = ctrlplane_job_agent.github.id # the ctrlplane job agent ID
    selector = "true"

    # All GitHub dispatch settings (installationId, owner, repo, workflowId,
    # ref) come from the referenced job agent above, so no overrides here.
    config = {}
  }
}
