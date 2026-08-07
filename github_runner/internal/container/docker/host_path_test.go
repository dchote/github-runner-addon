package docker

import (
	"context"
	"testing"
)

func TestSanitizeHostPath(t *testing.T) {
	if _, err := sanitizeHostPath(""); err == nil {
		t.Fatal("empty")
	}
	if _, err := sanitizeHostPath("/"); err == nil {
		t.Fatal("root")
	}
	if _, err := sanitizeHostPath("relative"); err == nil {
		t.Fatal("relative")
	}
	if _, err := sanitizeHostPath("/tmp/../etc"); err == nil {
		t.Fatal("dotdot")
	}
	got, err := sanitizeHostPath("/srv/gha-work/lab")
	if err != nil || got != "/srv/gha-work/lab" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestHostDirBindRoot(t *testing.T) {
	cases := []struct {
		in       string
		wantRoot string
		wantRel  string
	}{
		{"/srv/gha-work/lab", "/srv", "gha-work/lab"},
		{"/srv", "/srv", "."},
		{"/mnt/data/x", "/mnt", "data/x"},
		{"/opt/runners/a", "/opt", "runners/a"},
		{"/custom/path", "/", "custom/path"},
	}
	for _, tc := range cases {
		root, rel := hostDirBindRoot(tc.in)
		if root != tc.wantRoot || rel != tc.wantRel {
			t.Fatalf("%s => %s/%s want %s/%s", tc.in, root, rel, tc.wantRoot, tc.wantRel)
		}
	}
}

func TestEnsureHostDirRejectsInvalid(t *testing.T) {
	c := &Client{}
	for _, p := range []string{"", "/", "relative", "/tmp/../etc", "/tmp/\x00x"} {
		if err := c.EnsureHostDir(context.Background(), p); err == nil {
			t.Fatalf("expected reject for %q", p)
		}
	}
}
