// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"fmt"
	"testing"
	"time"

	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	frameworkschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestPolicyRuleComputedAttributePlanModifiers(t *testing.T) {
	t.Parallel()

	ruleBlocks := []string{
		"version_selector",
		"version_cooldown",
		"deployment_window",
		"deployment_dependency",
		"verification",
		"gradual_rollout",
		"any_approval",
		"environment_progression",
		"plan_validation_opa",
	}

	var schemaResponse frameworkresource.SchemaResponse
	(&PolicyResource{}).Schema(
		context.Background(),
		frameworkresource.SchemaRequest{},
		&schemaResponse,
	)

	for _, blockName := range ruleBlocks {
		for _, attributeName := range []string{"id", "created_at"} {
			t.Run(blockName+"."+attributeName, func(t *testing.T) {
				block, ok := schemaResponse.Schema.Blocks[blockName].(frameworkschema.ListNestedBlock)
				if !ok {
					t.Fatalf("%s is not a list nested block", blockName)
				}

				attribute, ok := block.NestedObject.Attributes[attributeName].(frameworkschema.StringAttribute)
				if !ok {
					t.Fatalf("%s.%s is not a string attribute", blockName, attributeName)
				}
				if len(attribute.PlanModifiers) != 1 {
					t.Fatalf("%s.%s has %d plan modifiers, want 1", blockName, attributeName, len(attribute.PlanModifiers))
				}

				request := planmodifier.StringRequest{
					State: tfsdk.State{
						Raw: tftypes.NewValue(
							tftypes.Object{
								AttributeTypes: map[string]tftypes.Type{
									attributeName: tftypes.String,
								},
							},
							map[string]tftypes.Value{
								attributeName: tftypes.NewValue(tftypes.String, nil),
							},
						),
					},
					StateValue:  types.StringNull(),
					PlanValue:   types.StringUnknown(),
					ConfigValue: types.StringNull(),
				}
				response := planmodifier.StringResponse{
					PlanValue: request.PlanValue,
				}

				attribute.PlanModifiers[0].PlanModifyString(context.Background(), request, &response)

				if !response.PlanValue.IsUnknown() {
					t.Fatalf(
						"%s.%s planned value is %s, want unknown for a new nested rule",
						blockName,
						attributeName,
						response.PlanValue,
					)
				}

				request.State.Raw = tftypes.NewValue(
					tftypes.Object{
						AttributeTypes: map[string]tftypes.Type{
							attributeName: tftypes.String,
						},
					},
					map[string]tftypes.Value{
						attributeName: tftypes.NewValue(tftypes.String, "stable-value"),
					},
				)
				request.StateValue = types.StringValue("stable-value")
				response.PlanValue = types.StringUnknown()

				attribute.PlanModifiers[0].PlanModifyString(context.Background(), request, &response)

				if response.PlanValue.ValueString() != "stable-value" {
					t.Fatalf(
						"%s.%s planned value is %s, want prior non-null state",
						blockName,
						attributeName,
						response.PlanValue,
					)
				}
			})
		}
	}
}

func TestAccPolicyResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-policy-%d", time.Now().UnixNano())
	updatedName := name + "-updated"
	description := "Terraform acceptance test policy"
	updatedDescription := "Terraform acceptance test policy updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyResourceConfig(name, description, 100, true),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact(description),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("priority"),
						knownvalue.Int64Exact(100),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("enabled"),
						knownvalue.Bool(true),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("version_selector").AtSliceIndex(0).AtMapKey("selector"),
						knownvalue.StringExact("!version.tag.contains('-rc')"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("version_selector").AtSliceIndex(0).AtMapKey("description"),
						knownvalue.StringExact("No release candidates"),
					),
				},
			},
			{
				Config: testAccPolicyResourceConfig(updatedName, updatedDescription, 200, false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(updatedName),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact(updatedDescription),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("priority"),
						knownvalue.Int64Exact(200),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test",
						tfjsonpath.New("enabled"),
						knownvalue.Bool(false),
					),
				},
			},
		},
	})
}

func TestAccPolicyResource_addEnvironmentProgression(t *testing.T) {
	name := fmt.Sprintf("tf-acc-policy-progression-%d", time.Now().UnixNano())
	configWithoutProgression := testAccPolicyResourceEnvironmentProgressionConfig(name, false)
	configWithProgression := testAccPolicyResourceEnvironmentProgressionConfig(name, true)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: configWithoutProgression,
			},
			{
				Config: configWithProgression,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test_progression",
						tfjsonpath.New("environment_progression").AtSliceIndex(0).AtMapKey("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test_progression",
						tfjsonpath.New("environment_progression").AtSliceIndex(0).AtMapKey("created_at"),
						knownvalue.NotNull(),
					),
				},
			},
			{
				Config: configWithProgression,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccPolicyResourceSleepVerification(t *testing.T) {
	name := fmt.Sprintf("tf-acc-policy-sleep-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccPolicyResourceSleepVerificationConfig(name, 60),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test_sleep",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test_sleep",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test_sleep",
						tfjsonpath.New("verification").AtSliceIndex(0).AtMapKey("trigger_on"),
						knownvalue.StringExact("jobSuccess"),
					),
				},
			},
			{
				Config: testAccPolicyResourceSleepVerificationConfig(name, 120),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_policy.test_sleep",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
				},
			},
		},
	})
}

func testAccPolicyResourceEnvironmentProgressionConfig(name string, includeProgression bool) string {
	environmentProgression := ""
	if includeProgression {
		environmentProgression = `
  environment_progression {
    depends_on_environment_selector = "environment.name == 'qa'"
    minimum_success_percentage      = 80
  }
`
	}

	return fmt.Sprintf(`
%s
resource "ctrlplane_policy" "test_progression" {
  name     = %q
  selector = "true"
%s
}
`, testAccProviderConfig(), name, environmentProgression)
}

func testAccPolicyResourceSleepVerificationConfig(name string, durationSeconds int) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_policy" "test_sleep" {
  name     = %q
  selector = "true"

  verification {
    trigger_on = "jobSuccess"

    metric {
      name     = "wait-for-stabilization"
      interval = "30s"
      count    = 1

      success {
        condition = "result.ok == true"
      }

      sleep {
        duration_seconds = %d
      }
    }
  }
}
`, testAccProviderConfig(), name, durationSeconds)
}

func testAccPolicyResourceConfig(name, description string, priority int, enabled bool) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_policy" "test" {
  name        = %q
  description = %q
  priority    = %d
  enabled     = %t
  selector    = "deployment.name == '%s'"

  version_selector {
    selector    = "!version.tag.contains('-rc')"
    description = "No release candidates"
  }

  version_cooldown {
    duration = "1h"
  }

  deployment_window {
    duration_minutes = 480
    rrule            = "DTSTART:20000101T160000\nRRULE:FREQ=WEEKLY;WKST=MO;BYDAY=MO,TU,WE,TH,FR"
    timezone         = "America/New_York"
    allow_window     = true
  }

  verification {
    trigger_on = "jobSuccess"

    metric {
      name     = "Cluster Agent Deployment Available"
      interval = "30s"
      count    = 3

      success {
        condition = "true"
        threshold = 1
      }

      datadog {
        site     = "us5.datadoghq.com"
        interval = "1m"
        queries = {
          avail = "avg:kubernetes_state.deployment.replicas_available{kube_deployment:datadog-cluster-agent}"
        }
        api_key = "dummy"
        app_key = "dummy"
      }
    }
  }

  gradual_rollout {
    rollout_type        = "linear-normalized"
    time_scale_interval = 14400
  }

  any_approval {
    min_approvals = 1
  }

  environment_progression {
    depends_on_environment_selector = "environment.name == 'qa'"
    minimum_success_percentage      = 80
  }
}
`, testAccProviderConfig(), name, description, priority, enabled, name)
}
