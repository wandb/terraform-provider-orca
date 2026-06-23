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

func TestAccDeploymentVariableValueResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-varval-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create with a literal_value.
			{
				Config: testAccDeploymentVariableValueResourceConfigLiteral(name, 100, "us-east"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("priority"),
						knownvalue.Int64Exact(100),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("literal_value"),
						knownvalue.StringExact("us-east"),
					),
				},
			},
			// Gap-3 async coverage: refreshing state immediately after the create
			// step must produce no drift. The async read-after-write must leave the
			// resource readable right after apply.
			{
				RefreshState: true,
			},
			// ImportState: the resource supports import via passthrough id.
			{
				ResourceName:      "ctrlplane_deployment_variable_value.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// 2. Update step: change priority and literal_value.
			{
				Config: testAccDeploymentVariableValueResourceConfigLiteral(name, 200, "us-west"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("priority"),
						knownvalue.Int64Exact(200),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("literal_value"),
						knownvalue.StringExact("us-west"),
					),
				},
			},
		},
	})
}

func TestAccDeploymentVariableValueResource_reference(t *testing.T) {
	name := fmt.Sprintf("tf-acc-varval-ref-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentVariableValueResourceConfigReference(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("priority"),
						knownvalue.Int64Exact(50),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("reference_value").AtMapKey("reference"),
						knownvalue.StringExact("resource"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("reference_value").AtMapKey("path"),
						knownvalue.ListExact([]knownvalue.Check{
							knownvalue.StringExact("metadata"),
							knownvalue.StringExact("region"),
						}),
					),
				},
			},
		},
	})
}

// testAccDeploymentVariableValueResourceConfigLiteral builds the dependency
// chain (system -> deployment -> deployment_variable -> deployment_variable_value)
// using a literal_value.
func testAccDeploymentVariableValueResourceConfigLiteral(name string, priority int64, literal string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name = %q
}

resource "ctrlplane_deployment" "test" {
  name              = %q
  resource_selector = "resource.name == '%s'"
}

resource "ctrlplane_deployment_variable" "test" {
  deployment_id = ctrlplane_deployment.test.id
  key           = %q
  description   = "Terraform acceptance test variable"
}

resource "ctrlplane_deployment_variable_value" "test" {
  variable_id       = ctrlplane_deployment_variable.test.id
  priority          = %d
  resource_selector = "resource.name == '%s'"
  literal_value     = %q
}
`, testAccProviderConfig(), name, name+"-deployment", name, name, priority, name, literal)
}

// testAccDeploymentVariableValueResourceConfigReference builds the dependency
// chain using a reference_value instead of a literal_value.
func testAccDeploymentVariableValueResourceConfigReference(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name = %q
}

resource "ctrlplane_deployment" "test" {
  name              = %q
  resource_selector = "resource.name == '%s'"
}

resource "ctrlplane_deployment_variable" "test" {
  deployment_id = ctrlplane_deployment.test.id
  key           = %q
  description   = "Terraform acceptance test variable"
}

resource "ctrlplane_deployment_variable_value" "test" {
  variable_id = ctrlplane_deployment_variable.test.id
  priority    = 50
  reference_value = {
    reference = "resource"
    path      = ["metadata", "region"]
  }
}
`, testAccProviderConfig(), name, name+"-deployment", name, name)
}
