package steampipeconfig

import (
	"fmt"
	"github.com/turbot/go-kit/helpers"
	"github.com/turbot/pipe-fittings/constants"
	"strings"
)

func ValidateConnectionName(connectionName string) error {
	if helpers.StringSliceContains(constants.ReservedConnectionNames, connectionName) {
		return fmt.Errorf("'%s' is a reserved connection name", connectionName)
	}
	if strings.HasPrefix(connectionName, constants.ReservedConnectionNamePrefix) {
		return fmt.Errorf("invalid connection name '%s' - connection names cannot start with '%s'", connectionName, constants.ReservedConnectionNamePrefix)
	}
	return nil
}
