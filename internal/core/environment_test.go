package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentReportsMissingPathsWithoutCreatingThemAndRemovesWriteProbe(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "missing")
	report := InspectEnvironment("127.0.0.1:8787", root, Config{LogDir: root, BackupDir: missing})
	if !report.Directories[0].Writable || !report.Directories[0].Readable || report.Directories[2].Exists {
		t.Fatalf("directories: %+v", report.Directories)
	}
	files, err := os.ReadDir(root)
	if err != nil || len(files) != 0 {
		t.Fatalf("diagnostic left files or created missing directory: %v %v", files, err)
	}
}
