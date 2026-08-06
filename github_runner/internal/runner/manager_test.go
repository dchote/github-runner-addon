package runner

import (
	"testing"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/github"
)

func TestValidateExtraEnv(t *testing.T) {
	if err := validateExtraEnv(map[string]string{"FOO": "bar"}); err != nil {
		t.Fatal(err)
	}
	if err := validateExtraEnv(map[string]string{"RUNNER_TOKEN": "x"}); err == nil {
		t.Fatal("expected reserved key error")
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
