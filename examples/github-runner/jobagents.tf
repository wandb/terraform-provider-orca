resource "ctrlplane_job_agent" "this" {
  name = "github-runner"

  github {
    installation_id = 54476706
    owner           = "wandb"
    repo            = ""
  }
}
