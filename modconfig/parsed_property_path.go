package modconfig

import (
	"strings"

	"github.com/turbot/pipe-fittings/perr"
	"github.com/turbot/pipe-fittings/schema"
)

type ParsedPropertyPath struct {
	Mod          string
	ItemType     string
	Name         string
	PropertyPath []string
	// optional scope of this property path ("self")
	Scope    string
	Original string
}

func (p *ParsedPropertyPath) PropertyPathString() string {
	return strings.Join(p.PropertyPath, ".")
}

func (p *ParsedPropertyPath) ToParsedResourceName() *ParsedResourceName {
	return &ParsedResourceName{
		Mod:      p.Mod,
		ItemType: p.ItemType,
		Name:     p.Name,
	}
}

func (p *ParsedPropertyPath) ToResourceName() string {
	return BuildModResourceName(p.ItemType, p.Name)
}

func (p *ParsedPropertyPath) String() string {
	return p.Original
}

func ParseResourcePropertyPath(propertyPath string) (*ParsedPropertyPath, error) {
	res := &ParsedPropertyPath{Original: propertyPath}

	// valid property paths:
	// <mod>.<resource>.<name>.<property path...>
	// <resource>.<name>.<property path...>
	// so either the first or second slice must be a valid resource type

	//
	// unless they are some flowpipe resources:
	//
	// mod.trigger.trigger_type.trigger_name.<property_path>
	// trigger.trigger_type.trigger_name.<property_path>
	//
	// We can have trigger and integration in this current format

	parts := strings.Split(propertyPath, ".")
	if len(parts) < 2 {
		return nil, perr.BadRequestWithMessage("invalid property path: " + propertyPath)
	}

	// special case handling for runtime dependencies which may have use the "self" qualifier
	// const RuntimeDependencyDashboardScope = "self"
	if parts[0] == "self" {
		res.Scope = parts[0]
		parts = parts[1:]
	}

	if schema.IsValidResourceItemType(parts[0]) {
		// put empty mod as first part
		parts = append([]string{""}, parts...)
	}

	if len(parts) < 3 {
		return nil, perr.BadRequestWithMessage("invalid property path: " + propertyPath)
	}

	switch len(parts) {
	case 3:
		// no property path specified
		res.Mod = parts[0]
		res.ItemType = parts[1]
		res.Name = parts[2]
	default:
		if parts[1] == "integration" || parts[1] == "trigger" {
			res.Mod = parts[0]
			res.ItemType = parts[1]
			res.Name = parts[2] + "." + parts[3]
			if len(parts) > 4 {
				res.PropertyPath = parts[3:]
			}
		} else {
			res.Mod = parts[0]
			res.ItemType = parts[1]
			res.Name = parts[2]
			res.PropertyPath = parts[3:]
		}
	}

	if !schema.IsValidResourceItemType(res.ItemType) {
		return nil, perr.BadRequestWithMessage("invalid resource item type passed to ParseResourcePropertyPath: " + propertyPath)
	}

	return res, nil
}
