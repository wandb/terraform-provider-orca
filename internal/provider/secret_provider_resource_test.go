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
				Config: testAccSecretProviderResourceConfig(name, "aws-secrets-manager", `{"region":"us-east-1"}`, 1),
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
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("config_wo"),
						knownvalue.Null(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("config_wo_version"),
						knownvalue.Int64Exact(1),
					),
				},
			},
			{
				RefreshState: true,
				Check:        resource.TestCheckNoResourceAttr("ctrlplane_secret_provider.test", "config_wo"),
			},
			{
				Config:   testAccSecretProviderResourceConfig(name, "aws-secrets-manager", `{"region":"us-east-1"}`, 1),
				PlanOnly: true,
			},
			{
				Config: testAccSecretProviderResourceConfig(name, "aws-secrets-manager", `{"region":"us-west-2"}`, 2),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("type"),
						knownvalue.StringExact("aws-secrets-manager"),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("config_wo"),
						knownvalue.Null(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_secret_provider.test",
						tfjsonpath.New("config_wo_version"),
						knownvalue.Int64Exact(2),
					),
				},
			},
			{
				ResourceName:            "ctrlplane_secret_provider.test",
				ImportState:             true,
				ImportStateId:           name,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"config_wo_version"},
			},
		},
	})
}

func testAccSecretProviderResourceConfig(name, providerType, config string, configVersion int64) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_secret_provider" "test" {
  name              = %q
  type              = %q
  config_wo         = %q
  config_wo_version = %d
}
`, testAccProviderConfig(), name, providerType, config, configVersion)
}
