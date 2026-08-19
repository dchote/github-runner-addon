package docker

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

func TestDetachedTimeout(t *testing.T) {
	ctx, cancel := DetachedTimeout(0)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Fatal("expected deadline")
	}
	ctx2, cancel2 := DetachedContext()
	defer cancel2()
	if _, ok := ctx2.Deadline(); !ok {
		t.Fatal("DetachedContext deadline")
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

func TestIsLocalOnlyImage(t *testing.T) {
	cases := []struct {
		image string
		want  bool
	}{
		{"8wi-os-runner:local", true},
		{"myoung34/github-runner:local", true},
		{"myoung34/github-runner:latest", false},
		{"localhost:5000/foo:latest", false},
		{"alpine:3.20", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsLocalOnlyImage(tc.image); got != tc.want {
			t.Fatalf("IsLocalOnlyImage(%q)=%v want %v", tc.image, got, tc.want)
		}
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

func TestConsumeImagePull(t *testing.T) {
	ok := "{\"status\":\"Pulling from library/alpine\"}\n{\"status\":\"Digest: sha256:abc\"}\n"
	if err := consumeImagePull(strings.NewReader(ok)); err != nil {
		t.Fatal(err)
	}
	err := consumeImagePull(strings.NewReader("{\"error\":\"pull access denied\"}\n"))
	if err == nil || err.Error() != "pull access denied" {
		t.Fatalf("got %v", err)
	}
	err = consumeImagePull(strings.NewReader("{\"errorDetail\":{\"message\":\"manifest unknown\"}}\n"))
	if err == nil || err.Error() != "manifest unknown" {
		t.Fatalf("got %v", err)
	}
}
