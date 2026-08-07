package docker

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsContextError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("boom"), false},
		{context.Canceled, true},
		{context.DeadlineExceeded, true},
		{fmt.Errorf("create container: %w", context.Canceled), true},
		{fmt.Errorf(`Post "http://docker/containers/create": context canceled`), true},
		{fmt.Errorf("pull image: context deadline exceeded"), true},
		{errors.New("name conflict"), false},
	}
	for _, tc := range cases {
		if got := IsContextError(tc.err); got != tc.want {
			t.Fatalf("IsContextError(%v)=%v want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsConflict(t *testing.T) {
	if !IsConflict(errors.New("Conflict. The container name is already in use")) {
		t.Fatal("expected conflict")
	}
	if IsConflict(context.Canceled) {
		t.Fatal("canceled is not conflict")
	}
}

func TestValidateVolumeName(t *testing.T) {
	if err := validateVolumeName(""); err == nil {
		t.Fatal("expected empty name error")
	}
	if err := validateVolumeName("   "); err == nil {
		t.Fatal("expected whitespace name error")
	}
	if err := validateVolumeName("gha-runner-x-work"); err != nil {
		t.Fatal(err)
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

func TestNormalizeVolumeMountpoint(t *testing.T) {
	mp, err := normalizeVolumeMountpoint("vol", " /var/lib/docker/volumes/vol/_data ")
	if err != nil || mp != "/var/lib/docker/volumes/vol/_data" {
		t.Fatalf("mp=%q err=%v", mp, err)
	}
	if _, err := normalizeVolumeMountpoint("vol", ""); err == nil {
		t.Fatal("expected empty mountpoint error")
	}
	if _, err := normalizeVolumeMountpoint("vol", "   "); err == nil {
		t.Fatal("expected whitespace mountpoint error")
	}
}

func TestLabelsMatch(t *testing.T) {
	have := map[string]string{LabelManaged: "true", LabelID: "abc"}
	if !labelsMatch(have, map[string]string{LabelManaged: "true", LabelID: "abc"}) {
		t.Fatal("expected match on id")
	}
	if labelsMatch(have, map[string]string{LabelManaged: "true", LabelID: "other"}) {
		t.Fatal("expected mismatch on id")
	}
	if !labelsMatch(have, nil) {
		t.Fatal("nil want should accept managed")
	}
	if labelsMatch(map[string]string{}, map[string]string{LabelID: "abc"}) {
		t.Fatal("unmanaged should not match")
	}
}
