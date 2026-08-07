package runner

import (
	"errors"
	"testing"

	"github.com/dchote/github-runner-addon/internal/store"
)

func TestDefaultWorkdirHostPath(t *testing.T) {
	got := defaultWorkdirHostPath(store.Runner{ContainerName: "gha-runner-lab", Name: "lab"})
	if got != "/srv/gha-work/lab" {
		t.Fatalf("got %q", got)
	}
	// Name is normalized when container suffix unavailable.
	got = defaultWorkdirHostPath(store.Runner{Name: "My Runner"})
	if got != "/srv/gha-work/my-runner" {
		t.Fatalf("normalized name: %q", got)
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

func TestPlanWorkdirReconfigure(t *testing.T) {
	desired := "/srv/gha-work/lab"
	cases := []struct {
		name      string
		volExists bool
		agentWF   string
		agentErr  error
		wantNeeds bool
	}{
		{"missing volume", false, "", nil, true},
		{"no runner file", true, "", errNoRunnerConfig, true},
		{"read error", true, "", errors.New("boom"), true},
		{"empty folder", true, "", nil, true},
		{"mismatch", true, "/tmp/runner/work", nil, true},
		{"match", true, "/srv/gha-work/lab", nil, false},
		{"match cleaned", true, "/srv/gha-work/lab/", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan := planWorkdirReconfigure(tc.volExists, tc.agentWF, tc.agentErr, desired)
			if plan.Needs != tc.wantNeeds {
				t.Fatalf("Needs=%v want %v reason=%s", plan.Needs, tc.wantNeeds, plan.Reason)
			}
		})
	}
}

func TestWorkdirPathsMatch(t *testing.T) {
	if !workdirPathsMatch("/srv/a", "/srv/a/") {
		t.Fatal("expected match")
	}
	if workdirPathsMatch("/srv/a", "/srv/b") {
		t.Fatal("expected mismatch")
	}
}
