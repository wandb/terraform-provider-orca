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

func TestAccDeploymentDataSource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-depds-%d", time.Now().UnixNano())
	deploymentName := name + "-deployment"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentDataSourceConfig(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ctrlplane_deployment.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ctrlplane_deployment.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(deploymentName),
					),
					statecheck.ExpectKnownValue(
						"data.ctrlplane_deployment.test",
						tfjsonpath.New("slug"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ctrlplane_deployment.test",
						tfjsonpath.New("resource_selector"),
						knownvalue.StringExact(fmt.Sprintf("resource.name == '%s'", name)),
					),
				},
			},
		},
	})
}

func testAccDeploymentDataSourceConfig(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name = %q
}

resource "ctrlplane_deployment" "test" {
  name              = %q
  resource_selector = "resource.name == '%s'"
}

data "ctrlplane_deployment" "test" {
  name = ctrlplane_deployment.test.name

  depends_on = [ctrlplane_deployment.test]
}
`, testAccProviderConfig(), name, name+"-deployment", name)
}
