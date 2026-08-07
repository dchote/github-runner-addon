package runner

import (
	"testing"

	"github.com/dchote/github-runner-addon/internal/store"
)

func TestDefaultWorkdirHostPath(t *testing.T) {
	got := defaultWorkdirHostPath(store.Runner{ContainerName: "gha-runner-lab", Name: "lab"})
	if got != "/srv/gha-work/lab" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveWorkdirHostPathOverride(t *testing.T) {
	rec := store.Runner{
		ContainerName:   "gha-runner-lab",
		WorkdirHostPath: "/srv/gha-work/custom",
	}
	if got := resolveWorkdirHostPath(rec); got != "/srv/gha-work/custom" {
		t.Fatalf("got %q", got)
	}
}

func TestValidateWorkdirHostPathRejectsDockerVolumes(t *testing.T) {
	err := validateWorkdirHostPath("/var/lib/docker/volumes/gha-runner-lab-work/_data")
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestValidateWorkdirHostPathOK(t *testing.T) {
	if err := validateWorkdirHostPath("/srv/gha-work/lab"); err != nil {
		t.Fatal(err)
	}
	if err := validateWorkdirHostPath(""); err != nil {
		t.Fatal(err)
	}
}

func TestParseRunnerWorkFolder(t *testing.T) {
	wf, err := parseRunnerWorkFolder([]byte(`{"agentId":1,"workFolder":"/tmp/runner/work"}`))
	if err != nil || wf != "/tmp/runner/work" {
		t.Fatalf("wf=%q err=%v", wf, err)
	}
}

func TestNormalizeWorkdirHostPath(t *testing.T) {
	rec := store.Runner{ContainerName: "gha-runner-lab"}
	if got := normalizeWorkdirHostPath(rec, "/srv/gha-work/lab"); got != "" {
		t.Fatalf("default should normalize to empty, got %q", got)
	}
	if got := normalizeWorkdirHostPath(rec, "/srv/gha-work/other"); got != "/srv/gha-work/other" {
		t.Fatalf("got %q", got)
	}
}
