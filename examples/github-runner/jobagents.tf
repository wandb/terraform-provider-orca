resource "ctrlplane_job_agent" "this" {
  name = "github-runner"

  github {
    installation_id = 142358814
    owner           = "wandb"
    repo            = ""
  }
}
