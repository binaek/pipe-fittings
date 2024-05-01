package modinstaller

import (
	"fmt"
	"strings"

	"github.com/turbot/pipe-fittings/error_helpers"
	"github.com/turbot/pipe-fittings/modconfig"
	"github.com/turbot/pipe-fittings/parse"
)

func (i *ModInstaller) GetRequiredModVersionsFromArgs(modsArgs []string) (map[string]*modconfig.ModVersionConstraint, error) {
	var errors []error
	mods := make(map[string]*modconfig.ModVersionConstraint, len(modsArgs))
	for _, modArg := range modsArgs {
		// create mod version from arg
		// special case for file: prefix
		var modVersion *modconfig.ModVersionConstraint
		var err error
		if i.isFilePath(modArg) {
			// special case for file paths
			modVersion, err = i.newFilepathModVersionConstraint(modArg)
		} else {
			modVersion, err = modconfig.NewModVersionConstraint(modArg)
		}
		if err != nil {
			errors = append(errors, err)
			continue
		}
		// if we are updating there are a few checks we need to make
		if i.updating() {
			modVersion, err = i.getUpdateVersion(modArg, modVersion)
			if err != nil {
				errors = append(errors, err)
				continue
			}
		}
		if i.uninstalling() {
			// it is not valid to specify a mod version for uninstall
			if modVersion.HasVersion() {
				errors = append(errors, fmt.Errorf("invalid arg '%s' - cannot specify a version when uninstalling", modArg))
				continue
			}
		}

		mods[modVersion.Name] = modVersion
	}
	if len(errors) > 0 {
		return nil, error_helpers.CombineErrors(errors...)
	}
	return mods, nil
}

func (i *ModInstaller) newFilepathModVersionConstraint(arg string) (*modconfig.ModVersionConstraint, error) {
	// remove the file: prefix
	filePath := strings.TrimPrefix(arg, modconfig.FilePrefix)
	// try to load the mod definition
	modDef, err := parse.LoadModfile(filePath)
	if err != nil {
		return nil, err
	}
	return modconfig.NewFilepathModVersionConstraint(modDef), nil
}

func (i *ModInstaller) getUpdateVersion(modArg string, modVersion *modconfig.ModVersionConstraint) (*modconfig.ModVersionConstraint, error) {
	// verify the mod is already installed
	if i.installData.Lock.GetMod(modVersion.Name, i.workspaceMod) == nil {
		return nil, fmt.Errorf("cannot update '%s' as it is not installed", modArg)
	}

	// find the current dependency with this mod name
	// - this is what we will be using, to ensure we keep the same version constraint
	currentDependency := i.workspaceMod.GetModDependency(modVersion.Name)
	if currentDependency == nil {
		return nil, fmt.Errorf("cannot update '%s' as it is not a dependency of this workspace", modArg)
	}

	// it is not valid to specify a mod version - we will set the constraint from the modfile
	if modVersion.HasVersion() {
		return nil, fmt.Errorf("invalid arg '%s' - cannot specify a version when updating", modArg)
	}
	return currentDependency, nil
}
