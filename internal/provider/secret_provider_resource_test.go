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

func TestAccSecretProviderResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-secprov-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccSecretProviderResourceConfig(name, "aws-secrets-manager", `{"region":"us-east-1"}`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("type"),
						knownvalue.StringExact("aws-secrets-manager"),
					),
				},
			},
			// Gap: async/no-drift refresh right after create.
			{
				RefreshState: true,
			},
			// Update the config and type.
			{
				Config: testAccSecretProviderResourceConfig(name, "doppler", `{"project":"tf-acc"}`),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("type"),
						knownvalue.StringExact("doppler"),
					),
				},
			},
			// Import by name; config is write-only and cannot be recovered, so it is ignored on verify.
			{
				ResourceName:            "ctrlplane_secret_provider.test",
				ImportState:             true,
				ImportStateId:           name,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config"},
			},
		},
	})
}

func testAccSecretProviderResourceConfig(name, providerType, config string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_secret_provider" "test" {
  name   = %q
  type   = %q
  config = %q
}
`, testAccProviderConfig(), name, providerType, config)
}
