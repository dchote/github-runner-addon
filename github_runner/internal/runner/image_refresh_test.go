package runner

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
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

func TestImageRefreshSchedule(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	m := NewManager(st, nil, nil, "", false, "t")
	if m.ImageRefreshDays() != 7 {
		t.Fatalf("default days %d", m.ImageRefreshDays())
	}
	target, _ := refreshImageRef(m.Image, "")
	m.refreshMu.Lock()
	m.imageRefreshCheckedAtLocked(target, time.Now())
	if m.imageRefreshDueLocked(target) {
		m.refreshMu.Unlock()
		t.Fatal("a check from this week is not due")
	}
	m.refreshMu.Unlock()

	m2 := NewManager(st, nil, nil, "", false, "t")
	m2.SetImageRefreshInterval(24 * time.Hour)
	m2.refreshMu.Lock()
	m2.loadImageRefreshStateLocked()
	if m2.imageRefreshDueLocked(target) {
		m2.refreshMu.Unlock()
		t.Fatal("saved check should survive a restart")
	}
	m2.imageRefreshChecked[target] = time.Now().Add(-48 * time.Hour)
	if !m2.imageRefreshDueLocked(target) {
		m2.refreshMu.Unlock()
		t.Fatal("a check older than the interval is due")
	}
	m2.refreshMu.Unlock()

	m2.SetImageRefreshInterval(0)
	if m2.ImageRefreshDays() != 0 {
		t.Fatalf("disabled days %d", m2.ImageRefreshDays())
	}
	m2.refreshMu.Lock()
	if m2.imageRefreshDueLocked(target) {
		m2.refreshMu.Unlock()
		t.Fatal("disabled interval is never due")
	}
	m2.refreshMu.Unlock()
}

func TestImageRefreshDoesNotRecordCheckWhenInspectFails(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	if err := st.Add(store.Runner{
		ID: "id1", Name: "one", URL: "https://github.com/a/b", Scope: "repo",
		ContainerName: "gha-one", Image: DefaultRunnerImage,
	}); err != nil {
		t.Fatal(err)
	}
	m := NewManager(st, nil, nil, "", false, "t")
	m.inspectFn = func(context.Context, string) (docker.InspectInfo, error) {
		return docker.InspectInfo{}, errors.New("daemon down")
	}
	target, _ := refreshImageRef(m.Image, "")
	busy, failed, incomplete := m.refreshRunnerImages(context.Background())
	if busy || failed || !incomplete {
		t.Fatalf("busy=%v failed=%v incomplete=%v", busy, failed, incomplete)
	}
	m.refreshMu.Lock()
	if !m.imageRefreshDueLocked(target) {
		m.refreshMu.Unlock()
		t.Fatal("a failed inspect must not consume the weekly check")
	}
	m.refreshMu.Unlock()
}
