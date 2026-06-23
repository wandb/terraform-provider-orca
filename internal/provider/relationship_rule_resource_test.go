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

func TestAccRelationshipRuleResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-relrule-%d", time.Now().UnixNano())
	reference := name + "-ref"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// 1. Create with name, reference, matcher, description, metadata.
			{
				Config: testAccRelationshipRuleResourceConfig(
					name,
					reference,
					"Terraform acceptance test relationship rule",
					"source.kind == 'Deployment' && target.kind == 'Resource'",
					"create",
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("reference"),
						knownvalue.StringExact(reference),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("Terraform acceptance test relationship rule"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("matcher"),
						knownvalue.StringExact("source.kind == 'Deployment' && target.kind == 'Resource'"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("metadata").AtMapKey("phase"),
						knownvalue.StringExact("create"),
					),
				},
			},
			// 2. Gap-3 async coverage: refresh state right after create to surface
			//    any eventual-consistency drift between Create and Read.
			{
				RefreshState: true,
			},
			// 3. ImportState: the resource uses ImportStatePassthroughID on "id",
			//    so a plain ID import is supported. "matcher" is CEL-normalized on
			//    the server (whitespace collapsed) so its imported form can differ
			//    from config; ignore it on verify to avoid spurious import drift.
			{
				ResourceName:            "ctrlplane_relationship_rule.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"matcher"},
			},
			// 4. Update: change description, matcher, and metadata. Reference is
			//    held constant because it has RequiresReplace.
			{
				Config: testAccRelationshipRuleResourceConfig(
					name,
					reference,
					"Updated terraform acceptance test relationship rule",
					"source.kind == 'Deployment' && target.kind == 'Environment'",
					"update",
				),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("Updated terraform acceptance test relationship rule"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("matcher"),
						knownvalue.StringExact("source.kind == 'Deployment' && target.kind == 'Environment'"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_relationship_rule.test",
						tfjsonpath.New("metadata").AtMapKey("phase"),
						knownvalue.StringExact("update"),
					),
				},
			},
		},
	})
}

func testAccRelationshipRuleResourceConfig(name, reference, description, matcher, phase string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_relationship_rule" "test" {
  name        = %q
  reference   = %q
  description = %q
  matcher     = %q
  metadata = {
    phase = %q
  }
}
`, testAccProviderConfig(), name, reference, description, matcher, phase)
}
