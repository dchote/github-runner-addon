package runner

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/store"
)

type fakeWorkdirHost struct {
	mu sync.Mutex

	ensureDirs []string
	hostFiles  map[string][]byte // absolute host path -> content
	files      map[string][]byte // volume/path -> content
	removed    []string
	ensureErr  error
	readErr    error
	reads      int
}

func newFakeWorkdirHost() *fakeWorkdirHost {
	return &fakeWorkdirHost{
		files:     map[string][]byte{},
		hostFiles: map[string][]byte{},
	}
}

func (f *fakeWorkdirHost) key(vol, rel string) string { return vol + "/" + rel }

func (f *fakeWorkdirHost) EnsureHostDir(_ context.Context, hostPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ensureErr != nil {
		return f.ensureErr
	}
	f.ensureDirs = append(f.ensureDirs, hostPath)
	return nil
}

func (f *fakeWorkdirHost) WriteHostFile(_ context.Context, hostPath string, data []byte, _ os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hostFiles[hostPath] = append([]byte(nil), data...)
	return nil
}

func (f *fakeWorkdirHost) ReadHostFile(_ context.Context, hostPath string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.hostFiles[hostPath]
	if !ok {
		return nil, docker.ErrHostFileNotFound
	}
	return append([]byte(nil), b...), nil
}

func (f *fakeWorkdirHost) ChmodHostPath(_ context.Context, _ string, _ os.FileMode) error {
	return nil
}

func (f *fakeWorkdirHost) ReadVolumeFile(_ context.Context, volumeName, relPath string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads++
	if f.readErr != nil {
		return nil, f.readErr
	}
	b, ok := f.files[f.key(volumeName, relPath)]
	if !ok {
		return nil, docker.ErrVolumeFileNotFound
	}
	return append([]byte(nil), b...), nil
}

func (f *fakeWorkdirHost) RemoveVolumeFiles(_ context.Context, volumeName string, relPaths ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range relPaths {
		k := f.key(volumeName, p)
		delete(f.files, k)
		f.removed = append(f.removed, k)
	}
	return nil
}

func TestReadAgentWorkFolderMissingAndCache(t *testing.T) {
	fh := newFakeWorkdirHost()
	m := &Manager{workdirHost: fh, agentWF: map[string]agentWorkFolderCache{}}
	_, err := m.readAgentWorkFolder(context.Background(), "vol", false)
	if !errors.Is(err, errNoRunnerConfig) {
		t.Fatalf("err=%v", err)
	}
	// Second call hits cache (no second map miss needing files).
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/x"}`)
	_, err = m.readAgentWorkFolder(context.Background(), "vol", false)
	if !errors.Is(err, errNoRunnerConfig) {
		t.Fatalf("cached missing should stick until invalidate: %v", err)
	}
	m.invalidateAgentWorkFolder("vol")
	wf, err := m.readAgentWorkFolder(context.Background(), "vol", false)
	if err != nil || wf != "/srv/x" {
		t.Fatalf("wf=%q err=%v", wf, err)
	}
}

func TestReadAgentWorkFolderBypassCache(t *testing.T) {
	fh := newFakeWorkdirHost()
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/old"}`)
	m := &Manager{workdirHost: fh, agentWF: map[string]agentWorkFolderCache{"vol": {folder: "/srv/old"}}}
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/new"}`)
	wf, err := m.readAgentWorkFolder(context.Background(), "vol", true)
	if err != nil || wf != "/srv/new" {
		t.Fatalf("bypass: wf=%q err=%v", wf, err)
	}
	if fh.reads != 1 {
		t.Fatalf("reads=%d want 1", fh.reads)
	}
	// Cache-only still returns stale until bypass/invalidate.
	wf, err = m.readAgentWorkFolder(context.Background(), "vol", false)
	if err != nil || wf != "/srv/new" {
		t.Fatalf("after bypass cache updated: wf=%q err=%v", wf, err)
	}
}

func TestReadAgentWorkFolderDoesNotCacheHardErrors(t *testing.T) {
	fh := newFakeWorkdirHost()
	fh.readErr = errors.New("pull failed")
	m := &Manager{workdirHost: fh}
	if _, err := m.readAgentWorkFolder(context.Background(), "vol", false); err == nil {
		t.Fatal("expected error")
	}
	fh.readErr = nil
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/ok"}`)
	wf, err := m.readAgentWorkFolder(context.Background(), "vol", false)
	if err != nil || wf != "/srv/ok" {
		t.Fatalf("retry after hard error: wf=%q err=%v", wf, err)
	}
}

func TestClearRunnerConfigForReconfigure(t *testing.T) {
	fh := newFakeWorkdirHost()
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/tmp/x"}`)
	m := &Manager{workdirHost: fh, agentWF: map[string]agentWorkFolderCache{"vol": {folder: "/tmp/x"}}}
	if err := m.clearRunnerConfigForReconfigure(context.Background(), "vol"); err != nil {
		t.Fatal(err)
	}
	if len(fh.removed) != 1 || fh.removed[0] != "vol/.runner" {
		t.Fatalf("removed=%v", fh.removed)
	}
	if _, ok := m.agentWF["vol"]; ok {
		t.Fatal("cache should be invalidated")
	}
}

func TestVerifyAgentWorkdir(t *testing.T) {
	fh := newFakeWorkdirHost()
	rec := store.Runner{ContainerName: "gha-runner-lab", VolumeName: "vol"}
	desired := resolveWorkdirHostPath(rec)
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"` + desired + `"}`)
	m := &Manager{workdirHost: fh}
	if err := m.verifyAgentWorkdir(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyAgentWorkdirFailsMismatch(t *testing.T) {
	fh := newFakeWorkdirHost()
	rec := store.Runner{ContainerName: "gha-runner-lab", VolumeName: "vol"}
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/tmp/runner/work"}`)
	m := &Manager{workdirHost: fh, verifyTimeout: 50 * time.Millisecond}
	err := m.verifyAgentWorkdir(context.Background(), rec)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatalf("want ErrValidation, got %v", err)
	}
}

func TestVerifyAgentWorkdirIgnoresCanceledParent(t *testing.T) {
	fh := newFakeWorkdirHost()
	rec := store.Runner{ContainerName: "gha-runner-lab", VolumeName: "vol"}
	desired := resolveWorkdirHostPath(rec)
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"` + desired + `"}`)
	m := &Manager{workdirHost: fh}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.verifyAgentWorkdir(ctx, rec); err != nil {
		t.Fatal(err)
	}
}

func TestEnrichListUsesCacheOnly(t *testing.T) {
	fh := newFakeWorkdirHost()
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/gha-work/lab"}`)
	rec := store.Runner{ContainerName: "gha-runner-lab", VolumeName: "vol", Name: "lab"}
	m := &Manager{
		workdirHost: fh,
		agentWF:     map[string]agentWorkFolderCache{"vol": {folder: "/srv/gha-work/lab"}},
	}
	v := m.enrich(context.Background(), rec, enrichOpts{workdirDiag: false})
	if v.WorkdirAgent != "/srv/gha-work/lab" || v.WorkdirMismatch {
		t.Fatalf("list enrich: %+v", v)
	}
	if fh.reads != 0 {
		t.Fatalf("list must not read volume files, reads=%d", fh.reads)
	}
}

func TestEnrichGetBypassesCache(t *testing.T) {
	fh := newFakeWorkdirHost()
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/gha-work/lab"}`)
	rec := store.Runner{ContainerName: "gha-runner-lab", VolumeName: "vol", Name: "lab"}
	m := &Manager{
		workdirHost: fh,
		agentWF:     map[string]agentWorkFolderCache{"vol": {folder: "/tmp/runner/work"}},
	}
	v := m.enrich(context.Background(), rec, enrichOpts{workdirDiag: true})
	if fh.reads != 1 {
		t.Fatalf("get must live-read, reads=%d", fh.reads)
	}
	if v.WorkdirAgent != "/srv/gha-work/lab" || v.WorkdirMismatch {
		t.Fatalf("get enrich: agent=%q mismatch=%v", v.WorkdirAgent, v.WorkdirMismatch)
	}
}

func TestEnsurePersistenceHostDirs(t *testing.T) {
	fh := newFakeWorkdirHost()
	m := &Manager{workdirHost: fh}
	rec := store.Runner{
		ContainerName: "gha-runner-lab",
		Cache:         &store.CacheConfig{Enabled: true, Type: "bind", HostPath: "/mnt/cache/lab"},
	}
	workdir := resolveWorkdirHostPath(rec)
	if err := m.ensurePersistenceHostDirs(context.Background(), rec, workdir); err != nil {
		t.Fatal(err)
	}
	if len(fh.ensureDirs) < 2 {
		t.Fatalf("ensureDirs=%v", fh.ensureDirs)
	}
	want := map[string]bool{workdir: false, workdir + "/" + jobHooksDirRel: false, "/mnt/cache/lab": false}
	for _, d := range fh.ensureDirs {
		if _, ok := want[d]; ok {
			want[d] = true
		}
	}
	for d, ok := range want {
		if !ok {
			t.Fatalf("missing ensure %s in %v", d, fh.ensureDirs)
		}
	}
	if _, ok := fh.hostFiles[jobStatusHostPath(workdir)]; !ok {
		t.Fatal("expected seeded job status.json")
	}
}
