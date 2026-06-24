resource "ctrlplane_job_agent" "github" {
  name = "github-workflow-runner"

  github {
    installation_id = 142358814
    owner           = "wandb"
    repo            = "orca-gh-get-job-inputs-test"

    # workflow_id and ref are carried on the agent so the workflow dispatch
    # gets a numeric workflow ID (the GitHub dispatcher requires a number; the
    # workflow's string-only config map cannot supply one). ref is the git ref.
    workflow_id = 301102333 # .github/workflows/get-job-inputs-test.yaml
    ref         = "main"
  }
}
