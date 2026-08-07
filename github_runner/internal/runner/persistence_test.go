package runner

import (
	"strings"
	"testing"

	"github.com/docker/docker/api/types/mount"

	"github.com/dchote/github-runner-addon/internal/store"
)

func TestValidateCacheVolume(t *testing.T) {
	c := &store.CacheConfig{Enabled: true, Type: "volume", Target: "/cache"}
	if err := validateCache(c); err != nil {
		t.Fatal(err)
	}
	c.VolumeName = "bad name!"
	if err := validateCache(c); err == nil {
		t.Fatal("expected invalid volume name")
	}
}

func TestValidateCacheBind(t *testing.T) {
	c := &store.CacheConfig{Enabled: true, Type: "bind", HostPath: "/srv/runner-cache", Target: "/cache"}
	if err := validateCache(c); err != nil {
		t.Fatal(err)
	}
	c.HostPath = ""
	if err := validateCache(c); err == nil {
		t.Fatal("expected host_path required")
	}
	c.HostPath = "/etc"
	if err := validateCache(c); err == nil {
		t.Fatal("expected forbidden path")
	}
}

func TestValidateCacheForbiddenTargets(t *testing.T) {
	for _, target := range []string{"/etc", "/proc", "/sys", "/var/run/docker.sock", "/"} {
		c := &store.CacheConfig{Enabled: true, Type: "volume", Target: target}
		if err := validateCache(c); err == nil {
			t.Fatalf("expected forbidden target %q", target)
		}
	}
}

func TestValidateCacheRegistrationCollision(t *testing.T) {
	c := &store.CacheConfig{Enabled: true, Type: "volume", Target: configFilesDir}
	if err := validateCache(c); err == nil {
		t.Fatal("expected collision with registration dir")
	}
}

func TestBuildExtraMounts(t *testing.T) {
	rec := store.Runner{
		ContainerName: "gha-runner-lab",
		Cache: &store.CacheConfig{
			Enabled:  true,
			Type:     "volume",
			Target:   "/cache",
			ReadOnly: true,
		},
	}
	hp := "/srv/gha-work/lab"
	mounts := buildExtraMounts(rec, hp)
	if len(mounts) != 2 {
		t.Fatalf("len=%d", len(mounts))
	}
	if mounts[0].Type != mount.TypeVolume || mounts[0].Target != "/cache" || !mounts[0].ReadOnly {
		t.Fatalf("cache: %+v", mounts[0])
	}
	if mounts[1].Type != mount.TypeBind || mounts[1].Source != hp || mounts[1].Target != hp {
		t.Fatalf("workdir: %+v", mounts[1])
	}
}

func TestBuildExtraMountsEmptyWorkdir(t *testing.T) {
	rec := store.Runner{ContainerName: "gha-runner-lab"}
	mounts := buildExtraMounts(rec, "")
	if len(mounts) != 0 {
		t.Fatalf("expected no mounts without workdir bind, got %+v", mounts)
	}
}

func TestStopTimeoutSecs(t *testing.T) {
	if StopTimeoutSecs != 120 {
		t.Fatalf("StopTimeoutSecs=%d", StopTimeoutSecs)
	}
}

func TestBuildEnvWorkdir(t *testing.T) {
	m := &Manager{}
	env := m.buildEnv(store.Runner{Name: "n", Scope: "repo", URL: "https://github.com/a/b", Labels: []string{"self-hosted"}}, "", "", "/srv/gha-work/n")
	if !strings.Contains(strings.Join(env, "\n"), "RUNNER_WORKDIR=/srv/gha-work/n") {
		t.Fatal(env)
	}
}

func TestCacheVolumeRefs(t *testing.T) {
	runners := []store.Runner{
		{ContainerName: "gha-runner-a", Cache: &store.CacheConfig{Enabled: true, Type: "volume", VolumeName: "shared"}},
		{ContainerName: "gha-runner-b", Cache: &store.CacheConfig{Enabled: true, Type: "volume", VolumeName: "shared"}},
	}
	if n := cacheVolumeRefs(runners, "shared"); n != 2 {
		t.Fatalf("shared refs=%d", n)
	}
}

func TestLegacyWorkVolumeName(t *testing.T) {
	if got := legacyWorkVolumeName(store.Runner{ContainerName: "gha-runner-lab"}); got != "gha-runner-lab-work" {
		t.Fatalf("%s", got)
	}
}
