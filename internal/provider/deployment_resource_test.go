// Copyright IBM Corp. 2021, 2026

package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccDeploymentResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-dep-%d", time.Now().UnixNano())
	updatedName := name + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentResourceConfig(name, "successful", "value"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
				},
			},
			{
				Config: testAccDeploymentResourceConfig(updatedName, "failure", "updated"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(updatedName),
					),
				},
			},
		},
	})
}

func TestAccDeploymentResource_idempotent(t *testing.T) {
	name := fmt.Sprintf("tf-acc-dep-idem-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentResourceConfig(name, "successful", "value"),
			},
			{
				Config: testAccDeploymentResourceConfig(name, "successful", "value"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccDeploymentResource_githubBlock(t *testing.T) {
	name := fmt.Sprintf("tf-acc-dep-gh-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentResourceConfigGitHub(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment.test",
						tfjsonpath.New("github").AtMapKey("installation_id"),
						knownvalue.Int64Exact(12345),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment.test",
						tfjsonpath.New("github").AtMapKey("workflow_id"),
						knownvalue.Int64Exact(67890),
					),
				},
			},
			{
				Config: testAccDeploymentResourceConfigGitHub(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccDeploymentResource_argocdBlock(t *testing.T) {
	name := fmt.Sprintf("tf-acc-dep-argo-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentResourceConfigArgoCD(name),
			},
			{
				Config: testAccDeploymentResourceConfigArgoCD(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func testAccDeploymentResourceConfigArgoCD(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name = %q
}

resource "ctrlplane_deployment" "test" {
  name              = %q
  resource_selector = "resource.name == '%s'"

  argocd {
    api_key    = "super-secret-token"
    server_url = "https://argocd.example.com"
    template   = "guestbook"
  }
}
`, testAccProviderConfig(), name, name+"-deployment", name)
}

func testAccDeploymentResourceConfigGitHub(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name = %q
}

resource "ctrlplane_deployment" "test" {
  name              = %q
  resource_selector = "resource.name == '%s'"

  github {
    installation_id = 12345
    owner           = "acme"
    repo            = "app"
    ref             = "main"
    workflow_id     = 67890
  }
}
`, testAccProviderConfig(), name, name+"-deployment", name)
}

func testAccDeploymentResourceConfig(name string, status string, metadataValue string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name = %q
}

resource "ctrlplane_job_agent" "test" {
  name = %q

  test_runner {
    delay_seconds = 5
    status        = %q
  }
}

resource "ctrlplane_deployment" "test" {
  name      = %q
  metadata = {
    key = %q
  }

  resource_selector = "resource.name == '%s'"

  job_agent_selector = "jobAgent.id == \"${ctrlplane_job_agent.test.id}\""

  test_runner {
    delay_seconds = 10
  }
}
`, testAccProviderConfig(), name, name+"-ja", status, name, metadataValue, name)
}
