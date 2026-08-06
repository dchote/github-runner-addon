package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAddConflictByContainerName(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "runners.json"))
	r1 := Runner{
		ID: "1", Name: "My Runner", ContainerName: "gha-runner-my-runner",
		URL: "https://github.com/a/b", Scope: "repo", CreatedAt: time.Now().UTC(),
	}
	if err := s.Add(r1); err != nil {
		t.Fatal(err)
	}
	r2 := Runner{
		ID: "2", Name: "my-runner", ContainerName: "gha-runner-my-runner",
		URL: "https://github.com/a/b", Scope: "repo", CreatedAt: time.Now().UTC(),
	}
	if err := s.Add(r2); err != ErrConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestUpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "runners.json"))
	r := Runner{
		ID: "1", Name: "lab", ContainerName: "gha-runner-lab",
		URL: "https://github.com/a/b", Scope: "repo", Labels: []string{"self-hosted"},
		CreatedAt: time.Now().UTC(),
	}
	if err := s.Add(r); err != nil {
		t.Fatal(err)
	}
	r.Labels = []string{"self-hosted", "linux", "lab"}
	r.CPULimit = 1.5
	if err := s.Update(r); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Labels) != 3 || got.CPULimit != 1.5 || got.UpdatedAt.IsZero() {
		t.Fatalf("update failed: %+v", got)
	}
	if err := s.Readable(); err != nil {
		t.Fatal(err)
	}
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	if err := s.Delete("1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("1"); err != ErrNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestUpdateNotFound(t *testing.T) {
	dir := t.TempDir()
	s := New(filepath.Join(dir, "runners.json"))
	if err := s.Update(Runner{ID: "missing"}); err != ErrNotFound {
		t.Fatalf("got %v", err)
	}
}
