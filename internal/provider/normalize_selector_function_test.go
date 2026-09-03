// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

func TestNormalizeSelectorFunction(t *testing.T) {
	t.Parallel()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_8_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: `
					output "test" {
						value = provider::ctrlplane::normalize_selector(<<-EOT
							(
							  resource.name == 'forest' &&
							  resource.kind == 'cluster'
							)
						EOT
						)
					}
				`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownOutputValue(
						"test",
						knownvalue.StringExact("( resource.name == 'forest' && resource.kind == 'cluster' )"),
					),
				},
			},
		},
	})
}

func TestNormalizeSelectorFunctionRun(t *testing.T) {
	t.Parallel()

	testCases := map[string]struct {
		input    string
		expected string
	}{
		"multiline selector": {
			input: `
				(
				  resource.name == 'forest' &&
				  resource.kind == 'cluster'
				)
			`,
			expected: "( resource.name == 'forest' && resource.kind == 'cluster' )",
		},
		"mixed whitespace": {
			input:    "\tresource.name\t ==\r\n'forest'\f ",
			expected: "resource.name == 'forest'",
		},
		"only whitespace": {
			input:    " \t\r\n",
			expected: "",
		},
		"already normalized": {
			input:    "resource.name == 'forest'",
			expected: "resource.name == 'forest'",
		},
		"unicode whitespace at edges": {
			input:    "\u00a0resource.name == 'forest'\u00a0",
			expected: "resource.name == 'forest'",
		},
		"unicode whitespace inside selector": {
			input:    "resource.name\u00a0==\u00a0'forest'",
			expected: "resource.name\u00a0==\u00a0'forest'",
		},
	}

	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			request := function.RunRequest{
				Arguments: function.NewArgumentsData([]attr.Value{types.StringValue(testCase.input)}),
			}
			expected := function.RunResponse{
				Result: function.NewResultData(types.StringValue(testCase.expected)),
			}
			got := function.RunResponse{
				Result: function.NewResultData(types.StringUnknown()),
			}

			NewNormalizeSelectorFunction().Run(context.Background(), request, &got)

			if diff := cmp.Diff(expected, got); diff != "" {
				t.Errorf("unexpected response (-want +got):\n%s", diff)
			}
		})
	}
}
