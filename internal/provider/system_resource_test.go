// Copyright IBM Corp. 2021, 2026

package provider

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	apiv1 "buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go/ctrlplane/api/v1"
	connect "connectrpc.com/connect"
	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

func TestAccSystemResource(t *testing.T) {
	name := fmt.Sprintf("tf-acc-%d", time.Now().UnixNano())
	updatedName := name + "-updated"
	description := "Terraform acceptance test"
	updatedDescription := "Terraform acceptance test updated"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: testAccSystemResourceConfig(name, description),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_system.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_system.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(name),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_system.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact(description),
					),
				},
			},
			{
				Config: testAccSystemResourceConfig(updatedName, updatedDescription),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(
						"ctrlplane_system.test",
						tfjsonpath.New("id"),
						knownvalue.NotNull(),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_system.test",
						tfjsonpath.New("name"),
						knownvalue.StringExact(updatedName),
					),
					statecheck.ExpectKnownValue(
						"ctrlplane_system.test",
						tfjsonpath.New("description"),
						knownvalue.StringExact(updatedDescription),
					),
				},
			},
		},
	})
}

// TestAccSystemResource_disappears verifies the Connect NotFound handling: when
// the system is deleted out-of-band, the next Read must remove it from state
// (not error), producing a non-empty plan that would recreate it.
func TestAccSystemResource_disappears(t *testing.T) {
	name := fmt.Sprintf("tf-acc-disappear-%d", time.Now().UnixNano())

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             testAccSystemResourceConfig(name, "disappears test"),
				Check:              testAccDeleteSystemOutOfBand("ctrlplane_system.test"),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// testAccDeleteSystemOutOfBand deletes the system directly via the Connect
// client, then polls until the engine reports it gone. The poll is required
// because engine mutations are asynchronous — without it the framework's
// post-step refresh could still observe the system and the NotFound path would
// not be exercised.
func testAccDeleteSystemOutOfBand(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource %s not found in state", resourceName)
		}

		client, err := api.NewWorkspaceClient(os.Getenv("CTRLPLANE_URL"), os.Getenv("CTRLPLANE_API_KEY"), os.Getenv("CTRLPLANE_WORKSPACE"))
		if err != nil {
			return fmt.Errorf("failed to build client: %w", err)
		}

		ctx := context.Background()
		if _, err := client.System.DeleteSystem(ctx, connect.NewRequest(&apiv1.DeleteSystemRequest{
			WorkspaceId: client.WorkspaceID(),
			SystemId:    rs.Primary.ID,
		})); err != nil {
			return fmt.Errorf("out-of-band delete failed: %w", err)
		}

		deadline := time.Now().Add(2 * time.Minute)
		for {
			_, err := client.System.GetSystem(ctx, connect.NewRequest(&apiv1.GetSystemRequest{
				WorkspaceId: client.WorkspaceID(),
				SystemId:    rs.Primary.ID,
			}))
			if isNotFound(err) {
				return nil
			}
			if err != nil {
				return fmt.Errorf("polling after out-of-band delete: %w", err)
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("system %s still present after out-of-band delete", rs.Primary.ID)
			}
			time.Sleep(2 * time.Second)
		}
	}
}

func testAccSystemResourceConfig(name, description string) string {
	return fmt.Sprintf(`
%s
resource "ctrlplane_system" "test" {
  name        = %q
  description = %q
}
`, testAccProviderConfig(), name, description)
}
