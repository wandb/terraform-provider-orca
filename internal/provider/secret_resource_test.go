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

func TestAccSecretResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-secret-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretResourceConfig(name, "db/password", "v1"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_secret.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret.test",
						tfjsonpath.New("scope"),
						knownvalue.StringExact("workspace"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret.test",
						tfjsonpath.New("key"),
						knownvalue.StringExact("db/password"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret.test",
						tfjsonpath.New("version"),
						knownvalue.StringExact("v1"),
					),
				},
			},
			// Gap: async/no-drift refresh right after create.
			{
				RefreshState: true,
			},
			// Update mutable fields (key + version); scope is immutable.
			{
				Config: testAccSecretResourceConfig(name, "db/password", "v2"),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_secret.test",
						tfjsonpath.New("version"),
						knownvalue.StringExact("v2"),
					),
				},
			},
			// Import by id.
			{
				ResourceName:      "ctrlplane_secret.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func testAccSecretResourceConfig(name, key, version string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_secret_provider" "test" {
  name              = "%s-provider"
  type              = "aws-secrets-manager"
  config_wo         = %q
  config_wo_version = 1
}

resource "ctrlplane_secret" "test" {
  scope       = "workspace"
  name        = %q
  provider_id = ctrlplane_secret_provider.test.id
  path        = ["secret", "data"]
  key         = %q
  version     = %q
}
`, testAccProviderConfig(), name, `{"region":"us-east-1"}`, name, key, version)
}
