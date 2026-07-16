// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
)

func TestCELBackedSchemaAndModelCoverage(t *testing.T) {
	direct := []struct {
		name, attribute, field string
		resource               resource.Resource
		model                  any
	}{
		{"deployment", "resource_selector", "ResourceSelector", &DeploymentResource{}, DeploymentResourceModel{}},
		{"deployment variable value", "resource_selector", "ResourceSelector", &DeploymentVariableValueResource{}, DeploymentVariableValueResourceModel{}},
		{"environment", "resource_selector", "ResourceSelector", &EnvironmentResource{}, EnvironmentResourceModel{}},
		{"relationship rule", "matcher", "Cel", &RelationshipRuleResource{}, RelationshipRuleResourceModel{}},
		{"variable set", "selector", "Selector", &VariableSetResource{}, VariableSetResourceModel{}},
	}
	for _, tc := range direct {
		t.Run(tc.name, func(t *testing.T) {
			var resp resource.SchemaResponse
			tc.resource.Schema(context.Background(), resource.SchemaRequest{}, &resp)
			requireCELStringAttribute(t, resp.Schema.Attributes[tc.attribute])
			requireCELStringModelField(t, tc.model, tc.field)
		})
	}

	var policyResp resource.SchemaResponse
	(&PolicyResource{}).Schema(context.Background(), resource.SchemaRequest{}, &policyResp)
	versionBlock := policyResp.Schema.Blocks["version_selector"].(schema.ListNestedBlock)
	requireCELStringAttribute(t, versionBlock.NestedObject.Attributes["selector"])
	requireCELStringModelField(t, PolicyVersionSelector{}, "Selector")
	progressionBlock := policyResp.Schema.Blocks["environment_progression"].(schema.ListNestedBlock)
	requireCELStringAttribute(t, progressionBlock.NestedObject.Attributes["depends_on_environment_selector"])
	requireCELStringModelField(t, PolicyEnvironmentProgression{}, "DependsOnEnvironmentSelector")
}

func requireCELStringAttribute(t *testing.T, attribute schema.Attribute) {
	t.Helper()
	stringAttribute, ok := attribute.(schema.StringAttribute)
	if !ok {
		t.Fatalf("attribute type = %T, want schema.StringAttribute", attribute)
	}
	if _, ok := stringAttribute.CustomType.(CELStringType); !ok {
		t.Fatalf("CustomType = %T, want CELStringType", stringAttribute.CustomType)
	}
}

func requireCELStringModelField(t *testing.T, model any, fieldName string) {
	t.Helper()
	field, ok := reflect.TypeOf(model).FieldByName(fieldName)
	if !ok {
		t.Fatalf("model %T has no field %s", model, fieldName)
	}
	if got, want := field.Type, reflect.TypeOf(CELStringValue{}); got != want {
		t.Fatalf("%T.%s type = %s, want %s", model, fieldName, got, want)
	}
}
