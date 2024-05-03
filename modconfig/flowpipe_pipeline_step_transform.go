package modconfig

import (
	"reflect"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/turbot/go-kit/helpers"
	"github.com/turbot/pipe-fittings/v2/error_helpers"
	"github.com/turbot/pipe-fittings/v2/hclhelpers"
	"github.com/turbot/pipe-fittings/v2/schema"
	"github.com/zclconf/go-cty/cty"
)

type PipelineStepTransform struct {
	PipelineStepBase
	Value any `json:"value"`
}

func (p *PipelineStepTransform) Equals(iOther PipelineStep) bool {
	if p == nil && helpers.IsNil(iOther) {
		return true
	}

	if p == nil && !helpers.IsNil(iOther) || !helpers.IsNil(iOther) && p == nil {
		return false
	}

	other, ok := iOther.(*PipelineStepTransform)
	if !ok {
		return false
	}

	if !p.PipelineStepBase.Equals(&other.PipelineStepBase) {
		return false
	}

	if helpers.IsNil(p.Value) && !helpers.IsNil(other.Value) {
		return false
	}

	if !helpers.IsNil(p.Value) && helpers.IsNil(other.Value) {
		return false
	}

	return reflect.DeepEqual(p.Value, other.Value)
}

func (p *PipelineStepTransform) GetInputs(evalContext *hcl.EvalContext) (map[string]interface{}, error) {
	var value any

	if p.UnresolvedAttributes[schema.AttributeTypeValue] == nil {
		value = p.Value
	} else {
		var transformValueCtyValue cty.Value
		diags := gohcl.DecodeExpression(p.UnresolvedAttributes[schema.AttributeTypeValue], evalContext, &transformValueCtyValue)
		if diags.HasErrors() {
			return nil, error_helpers.BetterHclDiagsToError(p.Name, diags)
		}

		goVal, err := hclhelpers.CtyToGo(transformValueCtyValue)
		if err != nil {
			return nil, err
		}
		value = goVal
	}

	return map[string]interface{}{
		schema.AttributeTypeValue: value,
	}, nil
}

func (p *PipelineStepTransform) SetAttributes(hclAttributes hcl.Attributes, evalContext *hcl.EvalContext) hcl.Diagnostics {

	diags := p.SetBaseAttributes(hclAttributes, evalContext)

	for name, attr := range hclAttributes {
		switch name {
		case schema.AttributeTypeValue:
			val, stepDiags := dependsOnFromExpressions(attr, evalContext, p)
			if stepDiags.HasErrors() {
				diags = append(diags, stepDiags...)
				continue
			}

			if val != cty.NilVal {
				goVal, err := hclhelpers.CtyToGo(val)
				if err != nil {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "Unable to parse " + schema.AttributeTypeValue + " attribute to interface",
						Subject:  &attr.Range,
					})
				}

				p.Value = goVal
			}

		default:
			if !p.IsBaseAttribute(name) {
				diags = append(diags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "Unsupported attribute for Transform Step: " + attr.Name,
					Subject:  &attr.Range,
				})
			}
		}
	}

	return diags
}
