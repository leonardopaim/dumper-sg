package docker

import (
	"context"
	"dumpersg/internal/core"
	"reflect"
	"strings"
	"testing"
)

func TestEnvironmentReportsSelectedDaemonWithoutRunningContainersOrPullingImages(t *testing.T) {
	s := discoveryScenario{Native: true, OS: "linux", ID: "qa-daemon"}
	var calls [][]string
	e, err := detect(context.Background(), Options{Runtime: "native"}, "windows", discoveryFactory(&s, &calls))
	if err != nil {
		t.Fatal(err)
	}
	report := e.Environment(context.Background(), core.Config{BackupDir: t.TempDir(), DockerImage: "dumper:test", MySQLImage: "mysql:test", UseHostNetwork: true})
	if !report.Available || report.Runtime != "native" || report.DaemonID != "qa-daemon" || report.Engine != "Docker Desktop" || len(report.Images) != 2 || !strings.Contains(report.Network, "host.docker.internal") {
		t.Fatalf("environment: %+v", report)
	}
	for _, call := range calls {
		if !reflect.DeepEqual(call[:1], []string{"docker"}) || (call[1] != "info" && call[1] != "version" && call[1] != "image") {
			t.Fatalf("diagnostic mutated Docker: %v", call)
		}
	}
}
