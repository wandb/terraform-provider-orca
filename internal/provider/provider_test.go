// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"os"
	"testing"

	"github.com/ctrlplanedev/terraform-provider-ctrlplane/internal/api"
	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
)

// testAccProtoV6ProviderFactories is used to instantiate a provider during acceptance testing.
// The factory function is called for each Terraform CLI command to create a provider
// server that the CLI can connect to and interact with.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"ctrlplane": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccProtoV6ProviderFactoriesWithEcho includes the echo provider alongside the scaffolding provider.
// It allows for testing assertions on data returned by an ephemeral resource during Open.
// The echoprovider is used to arrange tests by echoing ephemeral data into the Terraform state.
// This lets the data be referenced in test assertions with state checks.
var testAccProtoV6ProviderFactoriesWithEcho = map[string]func() (tfprotov6.ProviderServer, error){
	"ctrlplane": providerserver.NewProtocol6WithError(New("test")()),
	"echo":      echoprovider.NewProviderServer(),
}

func testAccPreCheck(t *testing.T) {
	if os.Getenv("CTRLPLANE_API_KEY") == "" {
		t.Skip("CTRLPLANE_API_KEY must be set for acceptance tests")
	}

	if os.Getenv("CTRLPLANE_WORKSPACE") == "" {
		t.Skip("CTRLPLANE_WORKSPACE must be set for acceptance tests")
	}

	if os.Getenv("CTRLPLANE_URL") == "" {
		t.Skip("CTRLPLANE_URL must be set for acceptance tests")
	}

	client, err := api.NewWorkspaceClient(os.Getenv("CTRLPLANE_URL"), os.Getenv("CTRLPLANE_API_KEY"), os.Getenv("CTRLPLANE_WORKSPACE"))
	if err != nil {
		t.Skipf("Failed to create API client: %s", err.Error())
	}

	if client.ID == uuid.Nil {
		t.Skip("CTRLPLANE_WORKSPACE not found for acceptance tests")
	}
}

func testAccProviderConfig() string {
	return `
terraform {
  required_providers {
    ctrlplane = {
      source = "ctrlplanedev/ctrlplane"
    }
  }
}

provider "ctrlplane" {}
`
}
