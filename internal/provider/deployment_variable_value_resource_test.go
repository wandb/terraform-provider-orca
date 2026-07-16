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

func TestAccDeploymentVariableValueResource_multilineSelector(t *testing.T) {
	name := fmt.Sprintf("tf-acc-varval-multiline-%d", time.Now().UnixNano())
	initial := fmt.Sprintf("resource.name == %q\n  && resource.kind == \"service\"\n", name)
	updated := fmt.Sprintf("resource.name == %q\n  && resource.kind == \"worker\"\n", name)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccDeploymentVariableValueResourceConfigMultilineSelector(name, initial, 100),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("resource_selector"),
						knownvalue.StringExact(initial),
					),
				},
			},
			{RefreshState: true},
			{
				Config: testAccDeploymentVariableValueResourceConfigMultilineSelector(name, updated, 200),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("resource_selector"),
						knownvalue.StringExact(updated),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_deployment_variable_value.test",
						tfjsonpath.New("priority"),
						knownvalue.Int64Exact(200),
					),
				},
			},
			{RefreshState: true},
			{
				Config: testAccDeploymentVariableValueResourceConfigMultilineSelector(name, updated, 200),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
				},
			},
		},
	})
}

func TestAccDeploymentVariableValueResource_literalTypes(t *testing.T) {
	cases := []struct {
		name    string
		literal string
	}{
		{"int", "100"},
		{"float", "3.14"},
		{"whole_float", "3.0"},
		{"negative", "-42"},
		{"zero", "0"},
		{"bool", "true"},
		{"string", `"hello"`},
		{"numeric_string", `"100"`},
		{"object", `{ replicas = 3, region = "us-east", enabled = true }`},
		{"nested_object", `{ limits = { cpu = 2, memory = 4 }, replicas = 3 }`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			name := fmt.Sprintf("tf-acc-varval-%s-%d", tc.name, time.Now().UnixNano())
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testAccDeploymentVariableValueConfigRawLiteral(name, tc.literal),
					},
					{
						Config: testAccDeploymentVariableValueConfigRawLiteral(name, tc.literal),
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{
								plancheck.ExpectEmptyPlan(),
							},
						},
					},
				},
			})
		})
	}
}

// testAccDeploymentVariableValueConfigRawLiteral injects a raw (unquoted) HCL
// expression as literal_value so callers can exercise non-string types.
func testAccDeploymentVariableValueConfigRawLiteral(name, literalExpr string) string {
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
  variable_id   = ctrlplane_deployment_variable.test.id
  priority      = 100
  literal_value = %s
}
`, testAccProviderConfig(), name, name+"-deployment", name, name, literalExpr)
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

func testAccDeploymentVariableValueResourceConfigMultilineSelector(name, selector string, priority int64) string {
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
}
resource "ctrlplane_deployment_variable_value" "test" {
  variable_id = ctrlplane_deployment_variable.test.id
  priority    = %d
  resource_selector = <<EOT
%sEOT
  literal_value = "multiline"
}
`, testAccProviderConfig(), name, name+"-deployment", name, name, priority, selector)
}
