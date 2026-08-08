package runner

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/dchote/github-runner-addon/internal/store"
)

func TestListEmptyAndStatusCountsWithoutDocker(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	m := NewManager(st, nil, nil, "", false, "test")
	list, err := m.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("len=%d", len(list))
	}
	counts, err := m.StatusCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if counts["total"] != 0 {
		t.Fatalf("counts=%v", counts)
	}
}

func TestStatusCountsMarksUnknownWithoutDocker(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	rec := store.Runner{
		ID:            "id1",
		Name:          "lab",
		URL:           "https://github.com/a/b",
		Scope:         "repo",
		Labels:        []string{"self-hosted"},
		ContainerName: "gha-runner-lab",
		VolumeName:    "gha-runner-lab-data",
		Image:         DefaultRunnerImage,
	}
	if err := st.Add(rec); err != nil {
		t.Fatal(err)
	}
	m := NewManager(st, nil, nil, "", false, "test")
	counts, err := m.StatusCounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// Align with List enrich: nil Docker → unknown (not missing).
	if counts["total"] != 1 || counts["unknown"] != 1 || counts["missing"] != 0 {
		t.Fatalf("counts=%v", counts)
	}
}

func TestLockRunnerForgetRemovesIdleKey(t *testing.T) {
	m := NewManager(nil, nil, nil, "", false, "test")
	unlock := m.lockRunnerOpt("create:gha-runner-lab", true)
	unlock()
	if _, ok := m.runnerLocks.Load("create:gha-runner-lab"); ok {
		t.Fatal("expected create: key pruned after unlock")
	}
}

func TestLockRunnerSerializes(t *testing.T) {
	m := NewManager(nil, nil, nil, "", false, "test")
	unlock1 := m.lockRunner("r1")
	done := make(chan struct{})
	go func() {
		unlock2 := m.lockRunner("r1")
		close(done)
		unlock2()
	}()
	select {
	case <-done:
		t.Fatal("second lock acquired while first held")
	default:
	}
	unlock1()
	<-done
}
