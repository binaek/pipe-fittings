package dashboardassets

import (
	"context"
	"encoding/json"
	"github.com/turbot/pipe-fittings/app_specific"
	"log"
	"os"

	filehelpers "github.com/turbot/go-kit/files"
	"github.com/turbot/pipe-fittings/filepaths"
	"github.com/turbot/pipe-fittings/ociinstaller"
	"github.com/turbot/pipe-fittings/statushooks"
	"github.com/turbot/steampipe-plugin-sdk/v5/logging"
)

func Ensure(ctx context.Context) error {
	logging.LogTime("dashboardassets.Ensure start")
	defer logging.LogTime("dashboardassets.Ensure end")

	// load report assets versions.json
	versionFile, err := loadReportAssetVersionFile()
	if err != nil {
		return err
	}

	if versionFile.Version == app_specific.AppVersion.String() {
		return nil
	}

	statushooks.SetStatus(ctx, "Installing dashboard server…")

	reportAssetsPath := filepaths.EnsureDashboardAssetsDir()

	// remove the legacy report folder, if it exists
	if _, err := os.Stat(filepaths.LegacyDashboardAssetsDir()); !os.IsNotExist(err) {
		os.RemoveAll(filepaths.LegacyDashboardAssetsDir())
	}

	return ociinstaller.InstallAssets(ctx, reportAssetsPath)
}

type ReportAssetsVersionFile struct {
	Version string `json:"version"`
}

func loadReportAssetVersionFile() (*ReportAssetsVersionFile, error) {
	versionFilePath := filepaths.ReportAssetsVersionFilePath()
	if !filehelpers.FileExists(versionFilePath) {
		return &ReportAssetsVersionFile{}, nil
	}

	file, _ := os.ReadFile(versionFilePath)
	var versionFile ReportAssetsVersionFile
	if err := json.Unmarshal(file, &versionFile); err != nil {
		log.Println("[ERROR]", "Error while reading dashboard assets version file", err)
		return nil, err
	}

	return &versionFile, nil

}
