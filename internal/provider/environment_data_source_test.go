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

func TestAccEnvironmentDataSource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-envds-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccEnvironmentDataSourceConfig(name),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"data.ctrlplane_environment.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"data.ctrlplane_environment.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"data.ctrlplane_environment.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact("Terraform acceptance test environment data source"),
					),
					statecheck.ExpectKnownValue(
						"data.ctrlplane_environment.test",
						tfjsonpath.New("resource_selector"),
						knownvalue.StringExact(fmt.Sprintf("resource.name == '%s'", name)),
					),
				},
			},
		},
	})
}

func testAccEnvironmentDataSourceConfig(name string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name = %q
}

resource "ctrlplane_environment" "test" {
  name              = %q
  description       = "Terraform acceptance test environment data source"
  resource_selector = "resource.name == '%s'"
}

data "ctrlplane_environment" "test" {
  name = ctrlplane_environment.test.name

  depends_on = [ctrlplane_environment.test]
}
`, testAccProviderConfig(), name+"-system", name, name)
}
