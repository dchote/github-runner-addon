package runner

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dchote/github-runner-addon/internal/store"
)

func TestPlanRunnerRebuild(t *testing.T) {
	cases := []struct {
		name                                string
		active, stale, retarget, disabled   bool
		pat                                 bool
		recreate, reconfigure, retargetOnly bool
		warn                                bool
	}{
		{name: "current and updating"},
		{name: "stopped is left alone", stale: true, retarget: true, disabled: true, pat: true},
		{name: "stale image", active: true, stale: true, recreate: true},
		{name: "stale and disabled without pat", active: true, stale: true, disabled: true, recreate: true, warn: true},
		{name: "disabled with pat reconfigures", active: true, disabled: true, pat: true, recreate: true, reconfigure: true},
		{name: "record tag rolled forward", active: true, retarget: true, retargetOnly: true},
		{name: "disabled without pat warns", active: true, disabled: true, warn: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := planRunnerRebuild(tc.active, tc.stale, tc.retarget, tc.disabled, tc.pat)
			if got.Recreate != tc.recreate || got.Reconfigure != tc.reconfigure || got.Retarget != tc.retargetOnly || got.Warn != tc.warn {
				t.Fatalf("plan=%+v", got)
			}
		})
	}
}

func TestRefreshImageRef(t *testing.T) {
	const latest = "myoung34/github-runner:latest"
	ref, fleet := refreshImageRef(latest, "myoung34/github-runner:2.336.0")
	if ref != latest || !fleet {
		t.Fatalf("old tag ref=%s fleet=%v", ref, fleet)
	}
	ref, fleet = refreshImageRef(latest, "docker.io/myoung34/github-runner:2.338.0")
	if ref != latest || !fleet {
		t.Fatalf("docker.io ref=%s fleet=%v", ref, fleet)
	}
	ref, fleet = refreshImageRef("", "")
	if ref != DefaultRunnerImage || !fleet {
		t.Fatalf("default ref=%s fleet=%v", ref, fleet)
	}
	ref, fleet = refreshImageRef(latest, "ghcr.io/example/custom:1")
	if ref != "ghcr.io/example/custom:1" || fleet {
		t.Fatalf("custom ref=%s fleet=%v", ref, fleet)
	}
	ref, fleet = refreshImageRef(latest, "ci-builder:local")
	if ref != "ci-builder:local" || fleet {
		t.Fatalf("local ref=%s fleet=%v", ref, fleet)
	}
	ref, fleet = refreshImageRef("ci-builder:local", "myoung34/github-runner:2.336.0")
	if fleet {
		t.Fatalf("local target must not roll the fleet: %s", ref)
	}
}

func TestImageRepository(t *testing.T) {
	cases := map[string]string{
		"myoung34/github-runner:2.336.0":                 "myoung34/github-runner",
		"docker.io/myoung34/github-runner:latest":        "myoung34/github-runner",
		"myoung34/github-runner@sha256:0123456789abcdef": "myoung34/github-runner",
		"localhost:5000/foo:latest":                      "localhost:5000/foo",
		"ci-builder:local":                               "ci-builder",
	}
	for in, want := range cases {
		if got := imageRepository(in); got != want {
			t.Fatalf("imageRepository(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRunnerImageStale(t *testing.T) {
	const oldID = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const newID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdee"
	if !runnerImageStale(oldID, newID) {
		t.Fatal("expected stale")
	}
	if runnerImageStale(oldID, oldID) {
		t.Fatal("same id is not stale")
	}
	if runnerImageStale("myoung34/github-runner:latest", newID) {
		t.Fatal("image ref must not count as stale")
	}
	if runnerImageStale("", newID) || runnerImageStale(oldID, "") {
		t.Fatal("empty id must not count as stale")
	}
}

func TestRunnerAutoUpdateDisabled(t *testing.T) {
	if !runnerAutoUpdateDisabled([]byte(`{"workFolder":"/srv/x","disableUpdate":true}`)) {
		t.Fatal("expected camelCase disableUpdate")
	}
	bom := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"DisableUpdate":true}`)...)
	if !runnerAutoUpdateDisabled(bom) {
		t.Fatal("expected PascalCase with BOM")
	}
	if runnerAutoUpdateDisabled([]byte(`{"workFolder":"/srv/x"}`)) {
		t.Fatal("missing flag is enabled")
	}
	if runnerAutoUpdateDisabled([]byte(`{"disableUpdate":false}`)) {
		t.Fatal("false is enabled")
	}
	if runnerAutoUpdateDisabled(nil) || runnerAutoUpdateDisabled([]byte("not-json")) {
		t.Fatal("unreadable config must not force reconfigure")
	}
}

func TestBuildEnvLeavesAutoUpdateEnabled(t *testing.T) {
	m := &Manager{}
	env := m.buildEnv(store.Runner{
		Name:   "n",
		Scope:  "repo",
		URL:    "https://github.com/a/b",
		Labels: []string{"self-hosted"},
	}, "", "", "/srv/gha-work/n")
	joined := strings.Join(env, "\n")
	if bytes.Contains([]byte(joined), []byte("DISABLE_AUTO_UPDATE=")) {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "DISABLE_AUTOMATIC_DEREGISTRATION=true") {
		t.Fatal(joined)
	}
}
