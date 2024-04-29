package versionmap

import (
	"encoding/json"
	"fmt"
	"github.com/turbot/pipe-fittings/app_specific"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver/v3"
	filehelpers "github.com/turbot/go-kit/files"
	"github.com/turbot/pipe-fittings/error_helpers"
	"github.com/turbot/pipe-fittings/filepaths"
	"github.com/turbot/pipe-fittings/modconfig"
	"github.com/turbot/pipe-fittings/versionhelpers"
)

const WorkspaceLockStructVersion = 20220411

// WorkspaceLock is a map of ModVersionMaps items keyed by the parent mod whose dependencies are installed
type WorkspaceLock struct {
	WorkspacePath   string
	InstallCache    DependencyVersionMap
	MissingVersions DependencyVersionMap

	ModInstallationPath string
	installedMods       VersionListMap
}

// EmptyWorkspaceLock creates a new empty workspace lock based,
// sharing workspace path and installedMods with 'existingLock'
func EmptyWorkspaceLock(existingLock *WorkspaceLock) *WorkspaceLock {
	return &WorkspaceLock{
		WorkspacePath:       existingLock.WorkspacePath,
		ModInstallationPath: filepaths.WorkspaceModPath(existingLock.WorkspacePath),
		InstallCache:        make(DependencyVersionMap),
		MissingVersions:     make(DependencyVersionMap),
		installedMods:       existingLock.installedMods,
	}
}

func LoadWorkspaceLock(workspacePath string) (*WorkspaceLock, error) {
	var installCache = make(DependencyVersionMap)
	lockPath := filepaths.WorkspaceLockPath(workspacePath)
	if filehelpers.FileExists(lockPath) {
		fileContent, err := os.ReadFile(lockPath)
		if err != nil {
			slog.Debug("error reading lock file", "lockPath", lockPath, "error", err)
			return nil, err
		}
		err = json.Unmarshal(fileContent, &installCache)
		if err != nil {
			slog.Debug("failed to unmarshal  lock file", "lockPath", lockPath, "error", err)
			return nil, err
		}
	}
	res := &WorkspaceLock{
		WorkspacePath:       workspacePath,
		ModInstallationPath: filepaths.WorkspaceModPath(workspacePath),
		InstallCache:        installCache,
		MissingVersions:     make(DependencyVersionMap),
	}

	if err := res.getInstalledMods(); err != nil {
		return nil, err
	}

	// populate the MissingVersions
	// (this removes missing items from the install cache)
	res.setMissing()

	return res, nil
}

// getInstalledMods returns a map installed mods, and the versions installed for each
func (l *WorkspaceLock) getInstalledMods() error {

	var includes []string
	for _, modFileName := range app_specific.ModFileNames() {
		includes = append(includes, fmt.Sprintf("**/%s", modFileName))
	}

	// recursively search for all the mod.sp files under the .steampipe/mods folder, then build the mod name from the file path
	modFiles, err := filehelpers.ListFiles(l.ModInstallationPath, &filehelpers.ListOptions{
		Flags:   filehelpers.FilesRecursive,
		Include: includes,
	})

	if err != nil {
		return err
	}

	// create result map - a list of version for each mod
	installedMods := make(VersionListMap, len(modFiles))
	// collect errors
	var errors []error

	for _, modfilePath := range modFiles {
		// try to parse the mod name and version form the parent folder of the modfile
		modDependencyName, version, err := l.parseModPath(modfilePath)
		if err != nil {
			// if we fail to parse, just ignore this modfile
			// - it's parent is not a valid mod installation folder so it is probably a child folder of a mod
			continue
		}

		// ensure the dependency mod folder is correctly named
		// - for old versions of steampipe the folder name would omit the patch number
		if err := l.validateAndFixFolderNamingFormat(modDependencyName, version, modfilePath); err != nil {
			continue
		}

		// add this mod version to the map
		installedMods.Add(modDependencyName, version)
	}

	if len(errors) > 0 {
		return error_helpers.CombineErrors(errors...)
	}
	l.installedMods = installedMods
	return nil
}

func (l *WorkspaceLock) validateAndFixFolderNamingFormat(modName string, version *semver.Version, modfilePath string) error {
	// verify folder name is of correct format (i.e. including patch number)
	modDir := filepath.Dir(modfilePath)
	parts := strings.Split(modDir, "@")
	currentVersionString := parts[1]
	desiredVersionString := fmt.Sprintf("v%s", version.String())
	if desiredVersionString != currentVersionString {
		desiredDir := fmt.Sprintf("%s@%s", parts[0], desiredVersionString)
		slog.Debug("renaming dependency mod folder %s to %s", modDir, desiredDir)
		return os.Rename(modDir, desiredDir)
	}
	return nil
}

// GetUnreferencedMods returns a map of all installed mods which are not in the lock file
func (l *WorkspaceLock) GetUnreferencedMods() VersionListMap {
	var unreferencedVersions = make(VersionListMap)
	for name, versions := range l.installedMods {
		for _, version := range versions {
			if !l.ContainsModVersion(name, version) {
				unreferencedVersions.Add(name, version)
			}
		}
	}
	return unreferencedVersions
}

// identify mods which are in InstallCache but not installed
// move them from InstallCache into MissingVersions
func (l *WorkspaceLock) setMissing() {
	// create a map of full modname to bool to allow simple checking
	flatInstalled := l.installedMods.FlatMap()

	for parent, deps := range l.InstallCache {
		// deps is a map of dep name to resolved contraint list
		// flatten and iterate

		for name, resolvedConstraint := range deps {
			fullName := modconfig.BuildModDependencyPath(name, resolvedConstraint.Version)

			if !flatInstalled[fullName] {
				// get the mod name from the constraint (fullName includes the version)
				name := resolvedConstraint.Name
				// remove this item from the install cache and add into missing
				l.MissingVersions.AddDependency(name, resolvedConstraint.Alias, resolvedConstraint.Version, resolvedConstraint.Constraint, parent, resolvedConstraint.GitRef, resolvedConstraint.Commit)
				l.InstallCache[parent].Remove(name)
			}
		}
	}
}

// extract the mod name and version from the modfile path
func (l *WorkspaceLock) parseModPath(modfilePath string) (modDependencyName string, modVersion *semver.Version, err error) {
	modFullName, err := filepath.Rel(l.ModInstallationPath, filepath.Dir(modfilePath))
	if err != nil {
		return
	}
	return modconfig.ParseModDependencyPath(modFullName)
}

func (l *WorkspaceLock) Save() error {
	if len(l.InstallCache) == 0 {
		l.Delete() //nolint:errcheck // ignore error
		return nil
	}
	content, err := json.MarshalIndent(l.InstallCache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepaths.WorkspaceLockPath(l.WorkspacePath), content, 0644) //nolint:gosec // TODO: check file permission
}

// Delete deletes the lock file
func (l *WorkspaceLock) Delete() error {
	if filehelpers.FileExists(filepaths.WorkspaceLockPath(l.WorkspacePath)) {
		return os.Remove(filepaths.WorkspaceLockPath(l.WorkspacePath))
	}
	return nil
}

// DeleteMods removes mods from the lock file then, if it is empty, deletes the file
func (l *WorkspaceLock) DeleteMods(mods VersionConstraintMap, parent *modconfig.Mod) {
	for modName := range mods {
		if parentDependencies := l.InstallCache[parent.GetInstallCacheKey()]; parentDependencies != nil {
			parentDependencies.Remove(modName)
		}
	}
}

// GetMod looks for a lock file entry matching the given mod dependency name
// (e.g.github.com/turbot/steampipe-mod-azure-thrifty
func (l *WorkspaceLock) GetMod(modDependencyName string, parent *modconfig.Mod) *ResolvedVersionConstraint {
	parentKey := parent.GetInstallCacheKey()

	if parentDependencies := l.InstallCache[parentKey]; parentDependencies != nil {
		// look for this mod in the lock file entries for this parent
		return parentDependencies[modDependencyName]
	}
	return nil
}

// FindMod looks for a lock file entry matching the given mod dependency name for any parent
func (l *WorkspaceLock) FindMod(dependencyName string) []*ResolvedVersionConstraint {
	var res []*ResolvedVersionConstraint
	for _, deps := range l.InstallCache {
		if deps[dependencyName] != nil {
			res = append(res, deps[dependencyName])
		}
	}
	return res
}

// GetLockedModVersions builds a ResolvedVersionListMap with the resolved versions
// for each item of the given VersionConstraintMap found in the lock file
func (l *WorkspaceLock) GetLockedModVersions(mods VersionConstraintMap, parent *modconfig.Mod) (ResolvedVersionListMap, error) {
	var res = make(ResolvedVersionListMap)
	for name, constraint := range mods {
		resolvedConstraint, err := l.GetLockedModVersion(constraint, parent)
		if err != nil {
			return nil, err
		}
		if resolvedConstraint != nil {
			res.Add(name, resolvedConstraint)
		}
	}
	return res, nil
}

// GetLockedModVersion looks for a lock file entry for the given parent matching the required constraint and returns nil if not found
func (l *WorkspaceLock) GetLockedModVersion(requiredModVersion *modconfig.ModVersionConstraint, parent *modconfig.Mod) (*ResolvedVersionConstraint, error) {
	lockedVersion := l.GetMod(requiredModVersion.Name, parent)
	if lockedVersion == nil {
		return nil, nil
	}

	// verify the locked version satisfies the version constraint
	// TODO KAI FIX to use requiredModVersion.Check
	if !requiredModVersion.Constraint().Check(lockedVersion.Version) { // TODO KAI FIX
		return nil, nil
	}

	return lockedVersion, nil
}

// FindLockedModVersion looks for a lock file entry matching the required constraint and returns nil if not found
func (l *WorkspaceLock) FindLockedModVersion(requiredModVersion *modconfig.ModVersionConstraint) (*ResolvedVersionConstraint, error) {
	lockedVersions := l.FindMod(requiredModVersion.Name)

	// todo if there are mnore than 1 mod satiusfyinh constraint, pick newest???

	for _, lockedVersion := range lockedVersions {
		// verify the locked version satisfies the version constraint
		// TODO KAI FIX to use requiredModVersion.Check
		if requiredModVersion.Constraint().Check(lockedVersion.Version) { // TODO KAI FIX

			return lockedVersion, nil
		}
	}
	return nil, nil
}

// EnsureLockedModVersion looks for a lock file entry matching the required mod name
func (l *WorkspaceLock) EnsureLockedModVersion(requiredModVersion *modconfig.ModVersionConstraint, parent *modconfig.Mod) (*ResolvedVersionConstraint, error) {
	lockedVersion := l.GetMod(requiredModVersion.Name, parent)
	if lockedVersion == nil {
		return nil, nil
	}

	// verify the locked version satisfies the version constraint
	if !requiredModVersion.Constraint().Check(lockedVersion.Version) { // TODO KAI FIX
		return nil, fmt.Errorf("failed to resolve dependencies for %s - locked version %s does not meet the constraint %s", parent.GetInstallCacheKey(), modconfig.BuildModDependencyPath(requiredModVersion.Name, lockedVersion.Version), "") //TODO requiredModVersion.Constraint.Original)
	}

	return lockedVersion, nil
}

// GetLockedModVersionConstraint looks for a lock file entry matching the required mod version and if found,
// returns it in the form of a ModVersionConstraint
func (l *WorkspaceLock) GetLockedModVersionConstraint(requiredModVersion *modconfig.ModVersionConstraint, parent *modconfig.Mod) (*modconfig.ModVersionConstraint, error) {
	lockedVersion, err := l.EnsureLockedModVersion(requiredModVersion, parent)
	if err != nil {
		// EnsureLockedModVersion returns an error if the locked version does not satisfy the requirement
		return nil, err
	}
	if lockedVersion == nil {
		// EnsureLockedModVersion returns nil if no locked version is found
		return nil, nil
	}
	// create a new ModVersionConstraint using the locked version
	lockedVersionFullName := modconfig.BuildModDependencyPath(requiredModVersion.Name, lockedVersion.Version)
	return modconfig.NewModVersionConstraint(lockedVersionFullName)
}

// ContainsModVersion returns whether the lockfile contains the given mod version
func (l *WorkspaceLock) ContainsModVersion(modName string, modVersion *semver.Version) bool {
	return l.SearchForModVersion(modName, modVersion) != nil
}

// TODO naming consistency
// SearchForModVersion returns the ResolvedVersionConstraint for the given mod version (if found)
func (l *WorkspaceLock) SearchForModVersion(modName string, modVersion *semver.Version) *ResolvedVersionConstraint {
	for _, modVersionMap := range l.InstallCache {
		for lockName, lockVersion := range modVersionMap {
			// TODO consider handling of metadata
			if lockName == modName && lockVersion.Version.Equal(modVersion) && lockVersion.Version.Metadata() == modVersion.Metadata() {
				return lockVersion
			}
		}
	}
	return nil
}

func (l *WorkspaceLock) ContainsModConstraint(modName string, constraint *versionhelpers.Constraints) bool {
	for _, modVersionMap := range l.InstallCache {
		for lockName, lockVersion := range modVersionMap {
			if lockName == modName && lockVersion.Constraint == constraint.Original {
				return true
			}
		}
	}
	return false
}

// Incomplete returned whether there are any missing dependencies
// (i.e. they exist in the lock file but ate not installed)
func (l *WorkspaceLock) Incomplete() bool {
	return len(l.MissingVersions) > 0
}

// Empty returns whether the install cache is empty
func (l *WorkspaceLock) Empty() bool {
	return l == nil || len(l.InstallCache) == 0
}

// StructVersion returns the struct version of the workspace lock
// because only the InstallCache is serialised, read the StructVersion from the first install cache entry
func (l *WorkspaceLock) StructVersion() int {
	for _, depVersionMap := range l.InstallCache {
		for _, depVersion := range depVersionMap {
			return depVersion.StructVersion
		}
	}
	// we have no deps - just return the new struct version
	return WorkspaceLockStructVersion

}

func (l *WorkspaceLock) FindInstalledDependency(modDependency *ResolvedVersionConstraint) (string, error) {
	dependencyFilepath := path.Join(l.ModInstallationPath, modDependency.DependencyPath())

	if filehelpers.DirectoryExists(dependencyFilepath) {
		return dependencyFilepath, nil
	}

	return "", fmt.Errorf("dependency mod '%s' is not installed - run '"+app_specific.AppName+" mod install'", modDependency.DependencyPath())
}
