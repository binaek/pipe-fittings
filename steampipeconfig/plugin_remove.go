package steampipeconfig

import (
	"fmt"
	"sort"
	"strings"

	"github.com/turbot/pipe-fittings/constants"
	"github.com/turbot/pipe-fittings/modconfig"
	"github.com/turbot/pipe-fittings/ociinstaller"
	"github.com/turbot/pipe-fittings/utils"
)

type PluginRemoveReport struct {
	Image       *ociinstaller.SteampipeImageRef
	ShortName   string
	Connections []*modconfig.Connection
}

type PluginRemoveReports []PluginRemoveReport

func (r PluginRemoveReports) Print() {
	length := len(r)
	var staleConnections []*modconfig.Connection
	if length > 0 {
		fmt.Printf("\nUninstalled %s:\n", utils.Pluralize("plugin", length)) //nolint:forbidigo // acceptable
		for _, report := range r {
			org, name, _ := report.Image.GetOrgNameAndStream()
			fmt.Printf("* %s/%s\n", org, name) //nolint:forbidigo // acceptable
			staleConnections = append(staleConnections, report.Connections...)

			// sort the connections by line number while we are at it!
			sort.SliceStable(report.Connections, func(i, j int) bool {
				left := report.Connections[i]
				right := report.Connections[j]
				return left.DeclRange.Start.Line < right.DeclRange.Start.Line
			})
		}
		fmt.Println() //nolint:forbidigo // acceptable
		staleLength := len(staleConnections)
		uniqueFiles := map[string]bool{}
		// get the unique files
		if staleLength > 0 {
			for _, report := range r {
				for _, conn := range report.Connections {
					uniqueFiles[conn.DeclRange.Filename] = true
				}
			}

			str := append([]string{}, fmt.Sprintf(
				"Please remove %s %s to continue using steampipe:",
				utils.Pluralize("this", len(uniqueFiles)),
				utils.Pluralize("connection", len(uniqueFiles)),
			))

			str = append(str, "")

			for file := range uniqueFiles {
				str = append(str, fmt.Sprintf("  * %s", constants.Bold(file)))
				for _, report := range r {
					for _, conn := range report.Connections {
						if conn.DeclRange.Filename == file {
							str = append(str, fmt.Sprintf("         '%s' (line %2d)", conn.Name, conn.DeclRange.Start.Line))
						}
					}
				}
				str = append(str, "")
			}

			fmt.Println(strings.Join(str, "\n")) //nolint:forbidigo // acceptable
		}
	}
}
