package runner

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/store"
)

type fakeWorkdirHost struct {
	mu sync.Mutex

	ensureDirs []string
	files      map[string][]byte // volume/path -> content
	removed    []string
	ensureErr  error
	readErr    error
}

func newFakeWorkdirHost() *fakeWorkdirHost {
	return &fakeWorkdirHost{files: map[string][]byte{}}
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

func (f *fakeWorkdirHost) ReadVolumeFile(_ context.Context, volumeName, relPath string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	_, err := m.readAgentWorkFolder(context.Background(), "vol")
	if !errors.Is(err, errNoRunnerConfig) {
		t.Fatalf("err=%v", err)
	}
	// Second call hits cache (no second map miss needing files).
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/x"}`)
	_, err = m.readAgentWorkFolder(context.Background(), "vol")
	if !errors.Is(err, errNoRunnerConfig) {
		t.Fatalf("cached missing should stick until invalidate: %v", err)
	}
	m.invalidateAgentWorkFolder("vol")
	wf, err := m.readAgentWorkFolder(context.Background(), "vol")
	if err != nil || wf != "/srv/x" {
		t.Fatalf("wf=%q err=%v", wf, err)
	}
}

func TestReadAgentWorkFolderDoesNotCacheHardErrors(t *testing.T) {
	fh := newFakeWorkdirHost()
	fh.readErr = errors.New("pull failed")
	m := &Manager{workdirHost: fh}
	if _, err := m.readAgentWorkFolder(context.Background(), "vol"); err == nil {
		t.Fatal("expected error")
	}
	fh.readErr = nil
	fh.files[fh.key("vol", runnerConfigFile)] = []byte(`{"workFolder":"/srv/ok"}`)
	wf, err := m.readAgentWorkFolder(context.Background(), "vol")
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
	m := &Manager{workdirHost: fh}
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	// Zero timeout: one attempt then ctx done or immediate deadline.
	err := m.verifyAgentWorkdir(ctx, rec)
	if err == nil {
		t.Fatal("expected failure")
	}
}
