package docker

import (
	"context"
	"errors"
	"testing"
)

func TestNormalizeVolumeRelPath(t *testing.T) {
	got, err := normalizeVolumeRelPath("/.runner")
	if err != nil || got != ".runner" {
		t.Fatalf("got %q err=%v", got, err)
	}
	if _, err := normalizeVolumeRelPath("../etc/passwd"); err == nil {
		t.Fatal("expected reject ..")
	}
	if _, err := normalizeVolumeRelPath(""); err == nil {
		t.Fatal("expected reject empty")
	}
}

func TestErrIfVolumeAbsent(t *testing.T) {
	if err := errIfVolumeAbsent(true, nil, ".runner", "vol"); err != nil {
		t.Fatal(err)
	}
	err := errIfVolumeAbsent(false, nil, ".runner", "vol")
	if !errors.Is(err, ErrVolumeFileNotFound) {
		t.Fatalf("missing volume must not proceed to helper create, got %v", err)
	}
	sentinel := errors.New("inspect failed")
	if err := errIfVolumeAbsent(false, sentinel, ".runner", "vol"); !errors.Is(err, sentinel) {
		t.Fatalf("got %v", err)
	}
}

func TestAcquireHelperNilSem(t *testing.T) {
	c := &Client{}
	rel, err := c.acquireHelper(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	rel()
}

func TestTarRegularFileRoundTrip(t *testing.T) {
	buf, err := tarRegularFile(".runner", []byte(`{"workFolder":"/srv/x"}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := readTarRegularFile(buf, ErrVolumeFileNotFound, ".runner")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"workFolder":"/srv/x"}` {
		t.Fatalf("got %q", got)
	}
}
