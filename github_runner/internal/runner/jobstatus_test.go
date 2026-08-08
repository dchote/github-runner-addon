package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dchote/github-runner-addon/internal/store"
)

func TestParseJobStatusFileIdleBusyUnknown(t *testing.T) {
	now := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)

	state, job := parseJobStatusFile([]byte(`{"busy":false,"updated_at":"2026-08-08T11:00:00Z"}`), now)
	if state != jobStateIdle || job != nil {
		t.Fatalf("idle: state=%q job=%v", state, job)
	}

	state, job = parseJobStatusFile([]byte(`{
		"busy":true,
		"repository":"o/r",
		"workflow":"CI",
		"job":"build",
		"run_id":"99",
		"updated_at":"2026-08-08T11:55:00Z"
	}`), now)
	if state != jobStateBusy || job == nil || job.Repository != "o/r" || job.Workflow != "CI" {
		t.Fatalf("busy: state=%q job=%+v", state, job)
	}

	state, job = parseJobStatusFile([]byte(`{"busy":true,"updated_at":"2026-08-01T12:00:00Z"}`), now)
	if state != jobStateUnknown || job != nil {
		t.Fatalf("stale busy: state=%q job=%v", state, job)
	}

	state, job = parseJobStatusFile([]byte(`{"busy":true}`), now)
	if state != jobStateUnknown || job != nil {
		t.Fatalf("busy without updated_at: state=%q job=%v", state, job)
	}

	state, job = parseJobStatusFile([]byte(`{"busy":true,"updated_at":"not-a-time"}`), now)
	if state != jobStateUnknown || job != nil {
		t.Fatalf("bad updated_at: state=%q job=%v", state, job)
	}

	state, job = parseJobStatusFile([]byte(`not-json`), now)
	if state != jobStateUnknown || job != nil {
		t.Fatalf("bad json: state=%q job=%v", state, job)
	}

	state, job = parseJobStatusFile(nil, now)
	if state != jobStateUnknown || job != nil {
		t.Fatalf("empty: state=%q job=%v", state, job)
	}
}

func TestBuildEnvJobHooks(t *testing.T) {
	m := &Manager{}
	env := m.buildEnv(
		store.Runner{Name: "n", Scope: "repo", URL: "https://github.com/a/b", Labels: []string{"self-hosted"}},
		"", "", "/srv/gha-work/n", false,
	)
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "ACTIONS_RUNNER_HOOK_JOB_STARTED=/srv/gha-work/n/.gha-addon/hooks/job-started.sh") {
		t.Fatal(joined)
	}
	if !strings.Contains(joined, "ACTIONS_RUNNER_HOOK_JOB_COMPLETED=/srv/gha-work/n/.gha-addon/hooks/job-completed.sh") {
		t.Fatal(joined)
	}
}

func TestValidateExtraEnvBlocksHookKeys(t *testing.T) {
	if err := validateExtraEnv(map[string]string{"ACTIONS_RUNNER_HOOK_JOB_STARTED": "/x"}); err == nil {
		t.Fatal("expected reserved key error")
	}
	if err := validateExtraEnv(map[string]string{"ACTIONS_RUNNER_HOOK_JOB_COMPLETED": "/x"}); err == nil {
		t.Fatal("expected reserved key error")
	}
}

func TestInstallJobHooksWritesFiles(t *testing.T) {
	fh := newFakeWorkdirHost()
	m := &Manager{workdirHost: fh}
	workdir := "/srv/gha-work/lab"
	if err := m.installJobHooks(context.Background(), workdir); err != nil {
		t.Fatal(err)
	}
	if _, ok := fh.hostFiles[jobStartedHookPath(workdir)]; !ok {
		t.Fatal("missing started hook")
	}
	if _, ok := fh.hostFiles[jobCompletedHookPath(workdir)]; !ok {
		t.Fatal("missing completed hook")
	}
	raw, ok := fh.hostFiles[jobStatusHostPath(workdir)]
	if !ok {
		t.Fatal("missing status.json")
	}
	state, job := parseJobStatusFile(raw, time.Now())
	if state != jobStateIdle || job != nil {
		t.Fatalf("seeded status: %q %+v", state, job)
	}
	foundHooksDir := false
	for _, d := range fh.ensureDirs {
		if d == workdir+"/"+jobHooksDirRel {
			foundHooksDir = true
		}
	}
	if !foundHooksDir {
		t.Fatalf("ensureDirs=%v", fh.ensureDirs)
	}
}

func TestEnsureJobHookScriptsDoesNotSeedStatus(t *testing.T) {
	fh := newFakeWorkdirHost()
	m := &Manager{workdirHost: fh}
	workdir := "/srv/gha-work/lab"
	if err := m.ensureJobHookScripts(context.Background(), workdir); err != nil {
		t.Fatal(err)
	}
	if _, ok := fh.hostFiles[jobStatusHostPath(workdir)]; ok {
		t.Fatal("scripts-only path must not write status.json")
	}
	if _, ok := fh.hostFiles[jobStartedHookPath(workdir)]; !ok {
		t.Fatal("missing started hook")
	}
}

func TestApplyJobStatusFromHostFile(t *testing.T) {
	fh := newFakeWorkdirHost()
	workdir := "/srv/gha-work/lab"
	fh.hostFiles[jobStatusHostPath(workdir)] = []byte(`{"busy":true,"repository":"a/b","workflow":"w","job":"j","updated_at":"2026-08-08T11:59:00Z"}`)
	m := &Manager{workdirHost: fh}
	v := &View{WorkdirEffective: workdir}
	m.applyJobStatus(context.Background(), v, "", workdir)
	if v.JobState != jobStateBusy || v.CurrentJob == nil || v.CurrentJob.Repository != "a/b" {
		t.Fatalf("%+v", v)
	}
}

func TestErrIfBusy(t *testing.T) {
	fh := newFakeWorkdirHost()
	workdir := "/srv/gha-work/lab"
	fh.hostFiles[jobStatusHostPath(workdir)] = []byte(`{"busy":true,"repository":"o/r","workflow":"Release","job":"build","updated_at":"2026-08-08T11:59:00Z"}`)
	m := &Manager{workdirHost: fh}
	rec := store.Runner{Name: "lab", WorkdirHostPath: workdir, ContainerName: "gha-runner-lab"}
	err := m.errIfBusy(context.Background(), rec)
	if err == nil || !errors.Is(err, ErrRunnerBusy) {
		t.Fatalf("want ErrRunnerBusy, got %v", err)
	}
	fh.hostFiles[jobStatusHostPath(workdir)] = []byte(`{"busy":false,"updated_at":"2026-08-08T11:59:00Z"}`)
	m.invalidateJobStatusCache(workdir)
	if err := m.errIfBusy(context.Background(), rec); err != nil {
		t.Fatalf("idle should allow: %v", err)
	}
}

func TestJobStatusCacheTTL(t *testing.T) {
	fh := newFakeWorkdirHost()
	workdir := "/srv/gha-work/lab"
	path := jobStatusHostPath(workdir)
	fh.hostFiles[path] = []byte(`{"busy":false,"updated_at":"2026-08-08T11:00:00Z"}`)
	m := &Manager{workdirHost: fh}
	v := &View{}
	m.applyJobStatus(context.Background(), v, "", workdir)
	if v.JobState != jobStateIdle {
		t.Fatalf("first read: %q", v.JobState)
	}
	fh.hostFiles[path] = []byte(`{"busy":true,"repository":"x/y","updated_at":"2026-08-08T11:59:00Z"}`)
	v2 := &View{}
	m.applyJobStatus(context.Background(), v2, "", workdir)
	if v2.JobState != jobStateIdle {
		t.Fatalf("cached read should still be idle, got %q", v2.JobState)
	}
	m.invalidateJobStatusCache(workdir)
	v3 := &View{}
	m.applyJobStatus(context.Background(), v3, "", workdir)
	if v3.JobState != jobStateBusy {
		t.Fatalf("after invalidate: %q", v3.JobState)
	}
}
