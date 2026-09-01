// Copyright IBM Corp. 2021, 2026
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/function"
)

var _ function.Function = &normalizeSelectorFunction{}

var selectorWhitespace = regexp.MustCompile(`\s+`)

type normalizeSelectorFunction struct{}

func NewNormalizeSelectorFunction() function.Function {
	return &normalizeSelectorFunction{}
}

func (f *normalizeSelectorFunction) Metadata(_ context.Context, _ function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = "normalize_selector"
}

func (f *normalizeSelectorFunction) Definition(_ context.Context, _ function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary: "Normalize selector whitespace",
		Description: "Trims whitespace from both ends of a selector and replaces each internal run of whitespace " +
			"with a single space. This is equivalent to replace(trimspace(selector), \"/\\\\s+/\", \" \").",
		Parameters: []function.Parameter{
			function.StringParameter{
				Name:        "selector",
				Description: "Selector expression to normalize.",
			},
		},
		Return: function.StringReturn{},
	}
}

func (f *normalizeSelectorFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	var selector string

	resp.Error = function.ConcatFuncErrors(resp.Error, req.Arguments.Get(ctx, &selector))
	if resp.Error != nil {
		return
	}

	result := selectorWhitespace.ReplaceAllString(strings.TrimSpace(selector), " ")
	resp.Error = function.ConcatFuncErrors(resp.Error, resp.Result.Set(ctx, result))
}
