package runner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
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
	if err := validateExtraEnv(map[string]string{"FOO": "bar\nbaz"}); err == nil {
		t.Fatal("expected newline in value")
	}
	if err := validateExtraEnv(map[string]string{"FOO": "bar\x00"}); err == nil {
		t.Fatal("expected NUL in value")
	}
}

func TestValidateNetworkMode(t *testing.T) {
	if err := validateNetworkMode(""); err != nil {
		t.Fatal(err)
	}
	if err := validateNetworkMode("bridge"); err != nil {
		t.Fatal(err)
	}
	if err := validateNetworkMode("container:abc123"); err != nil {
		t.Fatal(err)
	}
	if err := validateNetworkMode("container:"); err == nil {
		t.Fatal("expected empty container id")
	}
	if err := validateNetworkMode("weird"); err == nil {
		t.Fatal("expected reject")
	}
}

func TestValidateResourceLimits(t *testing.T) {
	if err := validateResourceLimits(0, 0); err != nil {
		t.Fatal(err)
	}
	if err := validateResourceLimits(-1, 0); err == nil {
		t.Fatal("expected negative cpu")
	}
	if err := validateResourceLimits(0, -1); err == nil {
		t.Fatal("expected negative memory")
	}
	if err := validateResourceLimits(maxCPULimit+1, 0); err == nil {
		t.Fatal("expected cpu cap")
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
	runEnv := m.buildEnv(rec, "", "", "/srv/gha-work/n")
	if docker.EnvHasKey(runEnv, "DEBUG_ONLY") {
		t.Fatal("run phase must not set DEBUG_ONLY")
	}
	if docker.EnvHasKey(runEnv, "RUNNER_TOKEN") {
		t.Fatal("run phase must not set RUNNER_TOKEN when empty")
	}
	cfgEnv := m.buildEnv(rec, "tok", "", "/srv/gha-work/n")
	joined := strings.Join(cfgEnv, "\n")
	if docker.EnvHasKey(cfgEnv, "DEBUG_ONLY") {
		t.Fatal("configure phase must not set DEBUG_ONLY (upstream skips config.sh)")
	}
	if !strings.Contains(joined, "RUNNER_TOKEN=tok") {
		t.Fatal(joined)
	}
}

func TestStartContainerWithoutVerifyConfigureCmd(t *testing.T) {
	fh := newFakeWorkdirHost()
	var got docker.CreateOpts
	m := &Manager{
		workdirHost: fh,
		createAndStartFn: func(_ context.Context, opts docker.CreateOpts) (string, error) {
			got = opts
			return "cid", nil
		},
	}
	rec := store.Runner{
		ID:            "id-1",
		Name:          "lab",
		Scope:         "repo",
		URL:           "https://github.com/a/b",
		Labels:        []string{"self-hosted"},
		ContainerName: "gha-runner-lab",
		VolumeName:    "vol",
		Image:         "myoung34/github-runner:latest",
	}
	if err := m.startContainerWithoutVerify(context.Background(), rec, "tok", "", true); err != nil {
		t.Fatal(err)
	}
	if len(got.Cmd) != 1 || got.Cmd[0] != configureNoopCmd {
		t.Fatalf("configure cmd=%v", got.Cmd)
	}
	if got.RestartPolicy != "no" {
		t.Fatalf("configure restart=%q", got.RestartPolicy)
	}
	if docker.EnvHasKey(got.Env, "DEBUG_ONLY") {
		t.Fatal("configure env must not set DEBUG_ONLY")
	}
	if !docker.EnvHasKey(got.Env, "RUNNER_TOKEN") {
		t.Fatal("configure env must set RUNNER_TOKEN")
	}
	if !got.AllowRunnerToken {
		t.Fatal("configure phase must allow adopting a leftover token container")
	}
	if err := m.startContainerWithoutVerify(context.Background(), rec, "", "", false); err != nil {
		t.Fatal(err)
	}
	if len(got.Cmd) != 0 {
		t.Fatalf("run phase cmd=%v", got.Cmd)
	}
	if got.RestartPolicy != "unless-stopped" {
		t.Fatalf("run restart=%q", got.RestartPolicy)
	}
	if docker.EnvHasKey(got.Env, "RUNNER_TOKEN") {
		t.Fatal("run phase must not set RUNNER_TOKEN")
	}
	if docker.EnvHasKey(got.Env, "DEBUG_ONLY") {
		t.Fatal("run phase must not set DEBUG_ONLY")
	}
	if got.AllowRunnerToken {
		t.Fatal("run phase must not adopt leftover RUNNER_TOKEN containers")
	}
}

func TestStartContainerImageNotFound(t *testing.T) {
	fh := newFakeWorkdirHost()
	m := &Manager{
		workdirHost: fh,
		createAndStartFn: func(_ context.Context, _ docker.CreateOpts) (string, error) {
			return "", fmt.Errorf("%w: 8wi-os-runner:local not found locally", docker.ErrImageNotFound)
		},
	}
	rec := store.Runner{
		Name: "os", Scope: "repo", URL: "https://github.com/a/b", Labels: []string{"self-hosted"},
		ContainerName: "gha-runner-os", VolumeName: "vol", Image: "8wi-os-runner:local",
	}
	err := m.startContainerWithoutVerify(context.Background(), rec, "tok", "", true)
	if err == nil || !errors.Is(err, ErrValidation) || !errors.Is(err, docker.ErrImageNotFound) {
		t.Fatalf("want validation+image not found, got %v", err)
	}
}

func TestStartContainerImagePull(t *testing.T) {
	fh := newFakeWorkdirHost()
	m := &Manager{
		workdirHost: fh,
		createAndStartFn: func(_ context.Context, _ docker.CreateOpts) (string, error) {
			return "", fmt.Errorf("%w: pull myoung34/github-runner:latest: connection refused", docker.ErrImagePull)
		},
	}
	rec := store.Runner{
		Name: "lab", Scope: "repo", URL: "https://github.com/a/b", Labels: []string{"self-hosted"},
		ContainerName: "gha-runner-lab", VolumeName: "vol", Image: "myoung34/github-runner:latest",
	}
	err := m.startContainerWithoutVerify(context.Background(), rec, "tok", "", true)
	if err == nil || !errors.Is(err, ErrImagePull) || errors.Is(err, ErrValidation) {
		t.Fatalf("want image pull (not validation), got %v", err)
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

func TestCreateRateLimitAfterValidation(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	m := NewManager(st, &docker.Client{}, nil, "img", false, "t")
	_, err := m.Create(context.Background(), CreateRequest{})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("empty create want validation, got %v", err)
	}
	for i := 0; i < 10; i++ {
		if !m.createLimiter.Allow() {
			t.Fatal("prefill limiter")
		}
	}
	_, err = m.Create(context.Background(), CreateRequest{})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("validation must not consume quota, got %v", err)
	}
	_, err = m.Create(context.Background(), CreateRequest{
		Name: "n", URL: "https://github.com/a/b", Token: "tok",
	})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("want rate limited after validation, got %v", err)
	}
}

func TestRollbackFailedCreateKeepsRegistration(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	rec := store.Runner{
		ID: "id1", Name: "lab", ContainerName: "gha-runner-lab", VolumeName: "vol",
		URL: "https://github.com/a/b", Scope: "repo", CreatedAt: time.Now().UTC(),
	}
	if err := st.Add(rec); err != nil {
		t.Fatal(err)
	}
	fh := newFakeWorkdirHost()
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/gha-work/lab"}`)
	m := &Manager{
		Store:       st,
		workdirHost: fh,
		agentWF:     map[string]agentWorkFolderCache{},
		inspectFn: func(_ context.Context, _ string) (docker.InspectInfo, error) {
			return docker.InspectInfo{Exists: false}, nil
		},
	}
	_, err := m.rollbackFailedCreate(rec, fmt.Errorf("%w: boom", ErrValidation))
	if err == nil {
		t.Fatal("expected create error")
	}
	if _, getErr := st.Get("id1"); getErr != nil {
		t.Fatalf("store row should remain: %v", getErr)
	}
}

func TestRollbackFailedCreateDeletesWhenNoArtifact(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	rec := store.Runner{
		ID: "id1", Name: "lab", ContainerName: "gha-runner-lab", VolumeName: "vol",
		URL: "https://github.com/a/b", Scope: "repo", CreatedAt: time.Now().UTC(),
	}
	if err := st.Add(rec); err != nil {
		t.Fatal(err)
	}
	m := &Manager{
		Store:       st,
		workdirHost: newFakeWorkdirHost(),
		agentWF:     map[string]agentWorkFolderCache{},
		inspectFn: func(_ context.Context, _ string) (docker.InspectInfo, error) {
			return docker.InspectInfo{Exists: false}, nil
		},
	}
	if _, err := m.rollbackFailedCreate(rec, fmt.Errorf("%w: boom", ErrValidation)); err == nil {
		t.Fatal("expected create error")
	}
	if _, err := st.Get("id1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("store row should be deleted, got %v", err)
	}
}

func TestBackupRestoreRunnerConfig(t *testing.T) {
	fh := newFakeWorkdirHost()
	orig := []byte(`{"workFolder":"/srv/gha-work/lab"}`)
	fh.files[fh.key("vol", runnerConfigFile)] = orig
	m := &Manager{workdirHost: fh, agentWF: map[string]agentWorkFolderCache{}}
	if err := m.backupRunnerConfig(context.Background(), "vol"); err != nil {
		t.Fatal(err)
	}
	if err := m.clearRunnerConfigForReconfigure(context.Background(), "vol"); err != nil {
		t.Fatal(err)
	}
	if _, err := fh.ReadVolumeFile(context.Background(), "vol", runnerConfigFile); !errors.Is(err, docker.ErrVolumeFileNotFound) {
		t.Fatalf("expected cleared .runner, got %v", err)
	}
	if err := m.restoreRunnerConfig(context.Background(), "vol"); err != nil {
		t.Fatal(err)
	}
	got, err := fh.ReadVolumeFile(context.Background(), "vol", runnerConfigFile)
	if err != nil || string(got) != string(orig) {
		t.Fatalf("restored=%q err=%v", got, err)
	}
}

func TestWaitForListeningTimeoutFailsClosed(t *testing.T) {
	m := &Manager{
		listenTimeout: 20 * time.Millisecond,
		inspectFn: func(_ context.Context, _ string) (docker.InspectInfo, error) {
			return docker.InspectInfo{Exists: true, Running: true, Status: "running"}, nil
		},
	}
	ok, err := m.waitForListening(context.Background(), "gha-runner-lab")
	if ok || !errors.Is(err, ErrValidation) {
		t.Fatalf("want validation timeout, confirmed=%v err=%v", ok, err)
	}
}

func TestPatchApplyDoesNotPersistOnFailure(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	rec := store.Runner{
		ID: "id1", Name: "lab", ContainerName: "gha-runner-lab",
		URL: "https://github.com/a/b", Scope: "repo", Labels: []string{"self-hosted"},
		CreatedAt: time.Now().UTC(),
	}
	if err := st.Add(rec); err != nil {
		t.Fatal(err)
	}
	m := NewManager(st, nil, nil, "img", false, "t")
	_, err := m.Patch(context.Background(), "id1", PatchRequest{
		Labels: []string{"self-hosted", "linux", "lab"},
		Apply:  true,
	})
	if !errors.Is(err, ErrDockerUnavailable) {
		t.Fatalf("got %v", err)
	}
	got, err := st.Get("id1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Labels) != 1 || got.Labels[0] != "self-hosted" {
		t.Fatalf("store must stay unchanged: %+v", got.Labels)
	}
}

func TestRecreateMissingEmpty(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	m := NewManager(st, nil, nil, "img", false, "t")
	out, err := m.RecreateMissing(context.Background(), RecreateMissingRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Recreated) != 0 || len(out.Failed) != 0 {
		t.Fatalf("%+v", out)
	}
}

func TestEnsureHooksThenStartMissing(t *testing.T) {
	m := &Manager{
		inspectFn: func(_ context.Context, _ string) (docker.InspectInfo, error) {
			return docker.InspectInfo{Exists: false}, nil
		},
	}
	err := m.ensureHooksThenStart(context.Background(), store.Runner{ContainerName: "gha-runner-lab", Name: "lab"})
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("got %v", err)
	}
}

func TestRedactSecrets(t *testing.T) {
	in := "token ghp_abc123 RUNNER_TOKEN=secret Bearer xyz github_pat_zzz"
	out := RedactSecrets(in)
	if strings.Contains(out, "ghp_abc123") || strings.Contains(out, "secret") || strings.Contains(out, "xyz") || strings.Contains(out, "zzz") {
		t.Fatalf("not redacted: %s", out)
	}
}
