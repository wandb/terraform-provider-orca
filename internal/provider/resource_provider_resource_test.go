// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

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

func TestAccResourceProviderResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-rp-%d", time.Now().UnixNano())
	updatedName := name + "-updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceProviderConfig_twoResources(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("metadata"),
						knownvalue.MapExact(map[string]knownvalue.Check{
							"managed_by": knownvalue.StringExact("terraform"),
						}),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource"),
						knownvalue.ListSizeExact(2),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(0).AtMapKey("name"),
						knownvalue.StringExact(name+"-res-a"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(0).AtMapKey("identifier"),
						knownvalue.StringExact(name+"-res-a"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(0).AtMapKey("kind"),
						knownvalue.StringExact("test/resource"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(0).AtMapKey("version"),
						knownvalue.StringExact("v1"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(1).AtMapKey("name"),
						knownvalue.StringExact(name+"-res-b"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(1).AtMapKey("identifier"),
						knownvalue.StringExact(name+"-res-b"),
					),
				},
			},
			{
				Config: testAccResourceProviderConfig_updatedResources(updatedName, name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(updatedName),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource"),
						knownvalue.ListSizeExact(2),
					),
					// res-a should still exist
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(0).AtMapKey("identifier"),
						knownvalue.StringExact(name+"-res-a"),
					),
					// res-b removed, res-c added
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(1).AtMapKey("identifier"),
						knownvalue.StringExact(name+"-res-c"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test",
						tfjsonpath.New("resource").AtSliceIndex(1).AtMapKey("kind"),
						knownvalue.StringExact("test/other"),
					),
				},
			},
		},
	})
}

func TestAccResourceProviderResourceEmpty(t *testing.T) {
	name := fmt.Sprintf("tf-acc-rp-empty-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceProviderConfig_noResources(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test_empty",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test_empty",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test_empty",
						tfjsonpath.New("resource"),
						knownvalue.ListSizeExact(0),
					),
				},
			},
			{
				Config: testAccResourceProviderConfig_addResource(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test_empty",
						tfjsonpath.New("resource"),
						knownvalue.ListSizeExact(1),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test_empty",
						tfjsonpath.New("resource").AtSliceIndex(0).AtMapKey("identifier"),
						knownvalue.StringExact(name+"-res"),
					),
				},
			},
		},
	})
}

// testAccResourceProviderConfig_twoResources creates a provider with two resources.
func testAccResourceProviderConfig_twoResources(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_resource_provider" "test" {
  name = %q

  metadata = {
    managed_by = "terraform"
  }

  resource {
    name       = %q
    identifier = %q
    kind       = "test/resource"
    version    = "v1"
    metadata = {
      env = "staging"
    }
  }

  resource {
    name       = %q
    identifier = %q
    kind       = "test/resource"
    version    = "v1"
    metadata = {
      env = "production"
    }
  }
}
`, testAccProviderConfig(), name,
		name+"-res-a", name+"-res-a",
		name+"-res-b", name+"-res-b")
}

// testAccResourceProviderConfig_updatedResources updates the provider name,
// keeps res-a, removes res-b, and adds res-c.
func testAccResourceProviderConfig_updatedResources(updatedName, originalName string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_resource_provider" "test" {
  name = %q

  metadata = {
    managed_by = "terraform"
  }

  resource {
    name       = %q
    identifier = %q
    kind       = "test/resource"
    version    = "v1"
    metadata = {
      env = "staging"
    }
  }

  resource {
    name       = %q
    identifier = %q
    kind       = "test/other"
    version    = "v2"
  }
}
`, testAccProviderConfig(), updatedName,
		originalName+"-res-a", originalName+"-res-a",
		originalName+"-res-c", originalName+"-res-c")
}

// testAccResourceProviderConfig_noResources creates a provider with no resources.
func testAccResourceProviderConfig_noResources(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_resource_provider" "test_empty" {
  name = %q
}
`, testAccProviderConfig(), name)
}

// testAccResourceProviderConfig_addResource adds a single resource to a previously empty provider.
func testAccResourceProviderConfig_addResource(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_resource_provider" "test_empty" {
  name = %q

  resource {
    name       = %q
    identifier = %q
    kind       = "test/resource"
    version    = "v1"
  }
}
`, testAccProviderConfig(), name, name+"-res", name+"-res")
}

func TestAccResourceProviderResource_idempotent(t *testing.T) {
	name := fmt.Sprintf("tf-acc-rp-idem-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceProviderConfig_twoResources(name),
			},
			{
				Config: testAccResourceProviderConfig_twoResources(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

func TestAccResourceProviderResource_configRoundTrip(t *testing.T) {
	name := fmt.Sprintf("tf-acc-rp-cfg-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceProviderConfig_withConfig(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_resource_provider.test_cfg",
						tfjsonpath.New("resource").AtSliceIndex(0).AtMapKey("config"),
						knownvalue.NotNull(),
					),
				},
			},
			{
				Config: testAccResourceProviderConfig_withConfig(name),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// testAccResourceProviderConfig_withConfig creates a provider with one resource carrying a JSON config.
func testAccResourceProviderConfig_withConfig(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_resource_provider" "test_cfg" {
  name = %q

  resource {
    name       = %q
    identifier = %q
    kind       = "test/resource"
    version    = "v1"
    config = jsonencode({
      replicas = 3
      ratio    = 0.5
      enabled  = true
      region   = "us-east-1"
      tags     = ["a", "b", "c"]
      nested = {
        cpu    = 2
        memory = "4Gi"
      }
    })
  }
}
`, testAccProviderConfig(), name, name+"-res", name+"-res")
}
