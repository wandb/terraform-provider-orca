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

func TestAccResourceResource(t *testing.T) {
	identifier := fmt.Sprintf("tf-acc-res-%d", time.Now().UnixNano())
	name := identifier + "-name"
	updatedName := name + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create.
			{
				Config: testAccResourceResourceConfig(identifier, name, "1.0.0", "infra"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact(identifier),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("identifier"),
						knownvalue.StringExact(identifier),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("kind"),
						knownvalue.StringExact("kubernetes/pod"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("version"),
						knownvalue.StringExact("1.0.0"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("metadata").AtMapKey("team"),
						knownvalue.StringExact("infra"),
					),
				},
			},
			// Gap-3 async coverage: refresh immediately after create to prove the
			// async read-after-write left no drift (resource is readable right away).
			{
				RefreshState: true,
			},
			// ImportState: import id is the identifier string.
			{
				ResourceName:      "ctrlplane_resource.test",
				ImportState:       true,
				ImportStateId:     identifier,
				ImportStateVerify: true,
			},
			// 2. Update (name/version/metadata change; identifier is RequiresReplace
			// and is held constant).
			{
				Config: testAccResourceResourceConfig(identifier, updatedName, "2.0.0", "platform"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("id"),
						knownvalue.StringExact(identifier),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(updatedName),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("version"),
						knownvalue.StringExact("2.0.0"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource.test",
						tfjsonpath.New("metadata").AtMapKey("team"),
						knownvalue.StringExact("platform"),
					),
				},
			},
		},
	})
}

func TestAccResourceResource_idempotent(t *testing.T) {
	identifier := fmt.Sprintf("tf-acc-res-idem-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceResourceConfig(identifier, identifier+"-name", "1.0.0", "infra"),
			},
			{
				Config: testAccResourceResourceConfig(identifier, identifier+"-name", "1.0.0", "infra"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccResourceResource_configTypes(t *testing.T) {
	identifier := fmt.Sprintf("tf-acc-res-cfg-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceResourceConfigRich(identifier),
			},
			{
				Config: testAccResourceResourceConfigRich(identifier),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// testAccResourceResourceConfigRich exercises the Dynamic config round-trip with
// mixed scalar types and a nested object.
func testAccResourceResourceConfigRich(identifier string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_resource" "test" {
  identifier = %q
  name       = %q
  version    = "1.0.0"
  kind       = "kubernetes/pod"

  config = {
    replicas = 3
    ratio    = 0.5
    enabled  = true
    region   = "us-east-1"
    nested = {
      cpu    = 2
      memory = "4Gi"
    }
  }
}
`, testAccProviderConfig(), identifier, identifier+"-name")
}

func testAccResourceResourceConfig(identifier, name, version, team string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_resource" "test" {
  identifier = %q
  name       = %q
  version    = %q
  kind       = "kubernetes/pod"

  config = {
    foo  = "bar"
    port = 8080
  }

  metadata = {
    team = %q
  }
}
`, testAccProviderConfig(), identifier, name, version, team)
}
