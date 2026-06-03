// Copyright IBM Corp. 2021, 2026

package provider

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccJobAgentResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-ja-%d", time.Now().UnixNano())
	updatedName := name + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccJobAgentResourceConfig(name, 5, "successful"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_job_agent.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_job_agent.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
				},
			},
			{
				Config: testAccJobAgentResourceConfig(updatedName, 10, "failure"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_job_agent.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_job_agent.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(updatedName),
					),
				},
			},
		},
	})
}

func testAccJobAgentResourceConfig(name string, delaySeconds int, status string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_job_agent" "test" {
  name = %q

  test_runner {
    delay_seconds = %d
    status        = %q
  }
}
`, testAccProviderConfig(), name, delaySeconds, status)
}

func TestAccJobAgentResource_HTTPPull(t *testing.T) {
	name := fmt.Sprintf("tf-acc-ja-pull-%d", time.Now().UnixNano())
	updatedName := name + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccJobAgentResourceConfigHTTPPull(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_job_agent.pull",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_job_agent.pull",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
				},
			},
			{
				Config: testAccJobAgentResourceConfigHTTPPull(updatedName),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_job_agent.pull",
						tfjsonpath.New("name"),
						knownvalue.StringExact(updatedName),
					),
				},
			},
		},
	})
}

func testAccJobAgentResourceConfigHTTPPull(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_job_agent" "pull" {
  name = %q

  http_pull {}
}
`, testAccProviderConfig(), name)
}
