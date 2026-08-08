package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/github"
	"github.com/dchote/github-runner-addon/internal/store"
)

func TestValidateExtraEnv(t *testing.T) {
	if err := validateExtraEnv(map[string]string{"FOO": "bar"}); err != nil {
		t.Fatal(err)
	}
	if err := validateExtraEnv(map[string]string{"RUNNER_TOKEN": "x"}); err == nil {
		t.Fatal("expected reserved key error")
	}
	if err := validateExtraEnv(map[string]string{"RUNNER_CACHE": "/scratch/x"}); err == nil {
		t.Fatal("expected reserved RUNNER_CACHE")
	}
}

func TestNormalizeLabels(t *testing.T) {
	got := normalizeLabels(nil)
	if len(got) != 2 || got[0] != "self-hosted" {
		t.Fatalf("%v", got)
	}
	got = normalizeLabels([]string{"  a ", "", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("%v", got)
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	if !rl.Allow() || !rl.Allow() {
		t.Fatal("first two should allow")
	}
	if rl.Allow() {
		t.Fatal("third should deny")
	}
}

func TestParseProjectURLViaGitHub(t *testing.T) {
	info, err := github.ParseProjectURL("https://github.com/example/my-repo")
	if err != nil || info.Scope != "repo" || info.Owner != "example" || info.Repo != "my-repo" {
		t.Fatalf("repo: %+v err=%v", info, err)
	}
	info, err = github.ParseProjectURL("https://github.com/my-org")
	if err != nil || info.Scope != "org" || info.OrgName() != "my-org" {
		t.Fatalf("org: %+v err=%v", info, err)
	}
	_, err = github.ParseProjectURL("https://github.com/")
	if err == nil {
		t.Fatal("expected validation error")
	}
	_, err = github.ParseProjectURL("https://github.com/owner/repo/tree/main")
	if err == nil {
		t.Fatal("expected validation error for deep paths")
	}
	info, err = github.ParseProjectURL("https://ghes.example.com/acme")
	if err != nil || info.Scope != "org" || info.Owner != "acme" || info.APIBase != "https://ghes.example.com/api/v3" {
		t.Fatalf("ghes org: %+v err=%v", info, err)
	}
}

func TestNormalizeStatus(t *testing.T) {
	cases := []struct {
		info        docker.InspectInfo
		wantStatus  string
		wantRunning bool
	}{
		{docker.InspectInfo{Exists: false}, "missing", false},
		{docker.InspectInfo{Exists: true, Status: "running", Running: true}, "running", true},
		{docker.InspectInfo{Exists: true, Status: "exited", Running: false}, "exited", false},
		{docker.InspectInfo{Exists: true, Status: "created", Running: false}, "exited", false},
		{docker.InspectInfo{Exists: true, Status: "paused", Running: false}, "exited", false},
		{docker.InspectInfo{Exists: true, Status: "dead", Running: false}, "exited", false},
		{docker.InspectInfo{Exists: true, Status: "restarting", Running: true}, "running", true},
		{docker.InspectInfo{Exists: true, Status: "weird", Running: false}, "unknown", false},
		{docker.InspectInfo{Exists: true, Status: "weird", Running: true}, "running", true},
	}
	for _, tc := range cases {
		status, running := normalizeStatus(tc.info)
		if status != tc.wantStatus || running != tc.wantRunning {
			t.Fatalf("%+v => %s/%v want %s/%v", tc.info, status, running, tc.wantStatus, tc.wantRunning)
		}
	}
}

func TestCountByStatus(t *testing.T) {
	counts := CountByStatus([]View{
		{Status: "running"},
		{Status: "running"},
		{Status: "missing"},
	})
	if counts["running"] != 2 || counts["missing"] != 1 || counts["total"] != 3 {
		t.Fatalf("%v", counts)
	}
}

func TestEnvHasKey(t *testing.T) {
	env := []string{"FOO=1", "RUNNER_TOKEN=secret"}
	if !docker.EnvHasKey(env, "RUNNER_TOKEN") {
		t.Fatal("expected token key")
	}
	if docker.EnvHasKey(env, "MISSING") {
		t.Fatal("unexpected key")
	}
}

func TestResolveRecreateTokenSkipsWhenNoReconfigure(t *testing.T) {
	m := &Manager{}
	tok, err := m.resolveRecreateToken(context.Background(), "https://github.com/a/b", "ignored", false)
	if err != nil || tok != "" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
}

func TestResolveRecreateTokenRequiresWhenReconfigure(t *testing.T) {
	m := &Manager{}
	_, err := m.resolveRecreateToken(context.Background(), "https://github.com/a/b", "", true)
	if err == nil {
		t.Fatal("expected error without PAT or token")
	}
	tok, err := m.resolveRecreateToken(context.Background(), "https://github.com/a/b", "reg-token", true)
	if err != nil || tok != "reg-token" {
		t.Fatalf("tok=%q err=%v", tok, err)
	}
}

func TestBuildEnvConfigureOnly(t *testing.T) {
	m := &Manager{}
	rec := store.Runner{Name: "n", Scope: "repo", URL: "https://github.com/a/b", Labels: []string{"self-hosted"}}
	runEnv := m.buildEnv(rec, "", "", "/srv/gha-work/n", false)
	if docker.EnvHasKey(runEnv, "DEBUG_ONLY") {
		t.Fatal("run phase must not set DEBUG_ONLY")
	}
	if docker.EnvHasKey(runEnv, "RUNNER_TOKEN") {
		t.Fatal("run phase must not set RUNNER_TOKEN when empty")
	}
	cfgEnv := m.buildEnv(rec, "tok", "", "/srv/gha-work/n", true)
	joined := strings.Join(cfgEnv, "\n")
	if !strings.Contains(joined, "DEBUG_ONLY=true") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "RUNNER_TOKEN=tok") {
		t.Fatal(joined)
	}
}

func TestRegistrationLogFailureSessionConflict(t *testing.T) {
	err := registrationLogFailure("√ Connected to GitHub\nA session for this runner already exists.\n")
	if err == nil {
		t.Fatal("expected session conflict failure")
	}
	if err := registrationLogFailure("√ Listening for Jobs"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateExtraEnvBlocksDebugOnly(t *testing.T) {
	if err := validateExtraEnv(map[string]string{"DEBUG_ONLY": "true"}); err == nil {
		t.Fatal("expected reserved key error")
	}
}
