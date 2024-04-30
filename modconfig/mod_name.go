package modconfig

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// BuildModDependencyPath converts a mod dependency name of form github.com/turbot/steampipe-mod-m2
// and a version into a dependency path of form github.com/turbot/steampipe-mod-m2@v1.0.0
func BuildModDependencyPath(dependencyName string, version *semver.Version) string {
	if version == nil {
		// not expected
		return dependencyName
	}

	return fmt.Sprintf("%s@v%s", dependencyName, version.String())
}

// BuildModBranchDependencyPath converts a mod dependency name of form github.com/turbot/steampipe-mod-m2
// and a branch into a dependency path of form github.com/turbot/steampipe-mod-m2#branch
func BuildModBranchDependencyPath(dependencyName string, branchName string) string {
	if branchName == "" {
		// not expected
		return dependencyName
	}

	return fmt.Sprintf("%s#%s", dependencyName, branchName)
}

// ParseModDependencyPath converts a mod depdency path of form github.com/turbot/steampipe-mod-m2@v1.0.0
// into the dependency name (github.com/turbot/steampipe-mod-m2) and version
func ParseModDependencyPath(fullName string) (string, *DependencyVersion, error) {
	switch {
	// is this a version constraint
	case strings.Contains(fullName, "@"):
		// split to get the name and version
		parts := strings.Split(fullName, "@")
		if len(parts) != 2 {
			err := fmt.Errorf("invalid mod full name %s", fullName)
			return "", nil, err
		}
		modDependencyName := parts[0]
		versionString := parts[1]
		version, err := semver.NewVersion(versionString)
		// NOTE: we expect the version to be in format 'vx.x.x', i.e. a semver with a preceding v
		if !strings.HasPrefix(versionString, "v") || err != nil {
			err = fmt.Errorf("mod file %s has invalid version", fullName)
		}
		modVersion := &DependencyVersion{
			Version: version,
		}
		return modDependencyName, modVersion, nil

		// branch constraint
	case strings.Contains(fullName, "#"):
		// split to get the name and branch
		parts := strings.Split(fullName, "#")
		if len(parts) != 2 {
			err := fmt.Errorf("invalid mod full name %s", fullName)
			return "", nil, err
		}
		modDependencyName := parts[0]
		branchName := parts[1]
		modVersion := &DependencyVersion{
			Branch: branchName,
		}
		return modDependencyName, modVersion, nil
		// TODO local filepath look for format modname:file/path

	}
	// TODO KAI: should we return an error here?
	return fullName, nil, nil
}
