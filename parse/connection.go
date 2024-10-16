package parse

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/turbot/pipe-fittings/hclhelpers"
	"github.com/turbot/pipe-fittings/modconfig"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
)

func DecodeConnection(block *hcl.Block) (*modconfig.SteampipeConnection, hcl.Diagnostics) {
	connectionContent, rest, diags := block.Body.PartialContent(ConnectionBlockSchema)
	if diags.HasErrors() {
		return nil, diags
	}

	connection := modconfig.NewConnection(block)

	// decode the plugin property
	// NOTE: this mutates connection to set PluginAlias and possible PluginInstance
	diags = decodeConnectionPluginProperty(connectionContent, connection)
	if diags.HasErrors() {
		return nil, diags
	}

	if connectionContent.Attributes["type"] != nil {
		var connectionType string
		diags = gohcl.DecodeExpression(connectionContent.Attributes["type"].Expr, nil, &connectionType)
		if diags.HasErrors() {
			return nil, diags
		}
		connection.Type = connectionType
	}
	if connectionContent.Attributes["import_schema"] != nil {
		var importSchema string
		diags = gohcl.DecodeExpression(connectionContent.Attributes["import_schema"].Expr, nil, &importSchema)
		if diags.HasErrors() {
			return nil, diags
		}
		connection.ImportSchema = importSchema
	}
	if connectionContent.Attributes["connections"] != nil {
		var connections []string
		diags = gohcl.DecodeExpression(connectionContent.Attributes["connections"].Expr, nil, &connections)
		if diags.HasErrors() {
			return nil, diags
		}
		connection.ConnectionNames = connections
	}

	// check for nested options
	for _, connectionBlock := range connectionContent.Blocks {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("connections do not support '%s' blocks", block.Type),
			Subject:  hclhelpers.BlockRangePointer(connectionBlock),
		})
	}

	if hclBody, ok := rest.(*hclsyntax.Body); ok {
		for _, b := range hclBody.Blocks {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("connections do not support '%s' blocks", b.Type),
				Subject:  hclhelpers.HclSyntaxBlockRangePointer(b),
			})
		}
	}

	// convert the remaining config to a hcl string to pass to the plugin
	config, moreDiags := hclhelpers.HclBodyToHclString(rest, connectionContent)
	if moreDiags.HasErrors() {
		diags = append(diags, moreDiags...)
	} else {
		connection.Config = config
	}

	return connection, diags
}

func decodeConnectionPluginProperty(connectionContent *hcl.BodyContent, connection *modconfig.SteampipeConnection) hcl.Diagnostics {
	var pluginName string
	evalCtx := &hcl.EvalContext{Variables: make(map[string]cty.Value)}

	diags := gohcl.DecodeExpression(connectionContent.Attributes["plugin"].Expr, evalCtx, &pluginName)
	res := NewDecodeResult()
	res.HandleDecodeDiags(diags)
	if res.Diags.HasErrors() {
		return res.Diags
	}
	if len(res.Depends) > 0 {
		slog.Info("decodeConnectionPluginProperty plugin property is HCL reference")
		// if this is a plugin reference, extract the plugin instance
		pluginInstance, ok := getPluginInstanceFromDependency(maps.Values(res.Depends))
		if !ok {
			slog.Info("failed to resolve plugin property")
			// return the original diagnostics
			return diags
		}

		// so we have resolved a reference to a plugin config
		// we will validate that this block exists later in initializePlugins
		// set PluginInstance ONLY
		// (the PluginInstance property being set means that we will raise the correct error if we fail to resolve the plugin block)
		connection.PluginInstance = &pluginInstance
		return nil
	}

	// NOTE: plugin property is set in initializePlugins
	connection.PluginAlias = pluginName

	return nil
}

func getPluginInstanceFromDependency(dependencies []*modconfig.ResourceDependency) (string, bool) {
	if len(dependencies) != 1 {
		return "", false
	}
	if len(dependencies[0].Traversals) != 1 {
		return "", false
	}
	traversalString := hclhelpers.TraversalAsString(dependencies[0].Traversals[0])
	split := strings.Split(traversalString, ".")
	if len(split) != 2 || split[0] != "plugin" {
		return "", false
	}
	return split[1], true
}
