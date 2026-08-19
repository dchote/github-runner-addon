package runner

import (
	"context"
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

func TestNormalizeCacheBindSamePath(t *testing.T) {
	c := normalizeCache(&store.CacheConfig{
		Enabled:  true,
		Type:     "bind",
		HostPath: "/scratch/build-cache/proj",
		Target:   "/cache",
	})
	if c == nil || c.HostPath != "/scratch/build-cache/proj" || c.Target != "/scratch/build-cache/proj" {
		t.Fatalf("expected same-path coerce, got %+v", c)
	}
	if c.VolumeName != "" {
		t.Fatalf("bind should clear volume_name: %+v", c)
	}
}

func TestResolveCacheEffective(t *testing.T) {
	if got := resolveCacheEffective(nil); got != "" {
		t.Fatalf("nil: %q", got)
	}
	bind := &store.CacheConfig{Enabled: true, Type: "bind", HostPath: "/scratch/x", Target: "/cache"}
	if got := resolveCacheEffective(bind); got != "/scratch/x" {
		t.Fatalf("bind: %q", got)
	}
	vol := &store.CacheConfig{Enabled: true, Type: "volume", Target: "/cache"}
	if got := resolveCacheEffective(vol); got != "/cache" {
		t.Fatalf("volume: %q", got)
	}
}

func TestCacheSiblingWarnings(t *testing.T) {
	if got := cacheSiblingWarnings(nil); got != nil {
		t.Fatalf("nil cache: %v", got)
	}
	same := &store.CacheConfig{Enabled: true, Type: "bind", HostPath: "/cache", Target: "/cache"}
	if got := cacheSiblingWarnings(same); got != nil {
		t.Fatalf("same-path bind should not warn: %v", got)
	}
	// After normalize, mismatch is coerced; raw mismatch also must not warn (binds never warn).
	mismatch := &store.CacheConfig{Enabled: true, Type: "bind", HostPath: "/srv/runner-cache", Target: "/cache"}
	if got := cacheSiblingWarnings(mismatch); got != nil {
		t.Fatalf("bind should not warn: %v", got)
	}
	vol := &store.CacheConfig{Enabled: true, Type: "volume", Target: "/cache"}
	gotVol := cacheSiblingWarnings(vol)
	if len(gotVol) != 1 || !strings.Contains(gotVol[0], "named volume") {
		t.Fatalf("expected named-volume sibling warning, got %v", gotVol)
	}
	if !strings.Contains(gotVol[0], "RUNNER_CACHE") {
		t.Fatalf("volume warning should mention RUNNER_CACHE: %s", gotVol[0])
	}
}

func TestEnrichAttachesCacheWarnings(t *testing.T) {
	m := &Manager{}
	rec := store.Runner{
		Name: "lab",
		Cache: &store.CacheConfig{
			Enabled:  true,
			Type:     "bind",
			HostPath: "/srv/runner-cache",
			Target:   "/cache",
		},
	}
	v := m.enrich(context.Background(), rec, enrichOpts{})
	if len(v.Warnings) != 0 {
		t.Fatalf("bind should have no warnings, got %v", v.Warnings)
	}
	if v.CacheEffective != "/srv/runner-cache" {
		t.Fatalf("cache_effective=%q", v.CacheEffective)
	}
	// Coerce-on-read: View cache.target matches host_path without store rewrite.
	if v.Cache == nil || v.Cache.Target != "/srv/runner-cache" {
		t.Fatalf("expected same-path coerce on View, got %+v", v.Cache)
	}
	if rec.Cache.Target != "/cache" {
		t.Fatalf("store record must stay unchanged, got target=%q", rec.Cache.Target)
	}

	rec.Cache = &store.CacheConfig{Enabled: true, Type: "volume", Target: "/cache"}
	v = m.enrich(context.Background(), rec, enrichOpts{})
	if len(v.Warnings) != 1 || !strings.Contains(v.Warnings[0], "named volume") {
		t.Fatalf("expected volume warning on View, got %v", v.Warnings)
	}
	if v.CacheEffective != "/cache" {
		t.Fatalf("volume cache_effective=%q", v.CacheEffective)
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

func TestBuildExtraMountsBindSamePath(t *testing.T) {
	rec := store.Runner{
		ContainerName: "gha-runner-lab",
		Cache: &store.CacheConfig{
			Enabled:  true,
			Type:     "bind",
			HostPath: "/scratch/build-cache/lab",
			Target:   "/cache", // ignored for mounts; same-path uses host
		},
	}
	mounts := buildExtraMounts(rec, "/srv/gha-work/lab")
	if len(mounts) != 2 {
		t.Fatalf("len=%d", len(mounts))
	}
	if mounts[0].Type != mount.TypeBind || mounts[0].Source != "/scratch/build-cache/lab" || mounts[0].Target != "/scratch/build-cache/lab" {
		t.Fatalf("cache bind: %+v", mounts[0])
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

func TestBuildEnvWorkdirAndCache(t *testing.T) {
	m := &Manager{}
	rec := store.Runner{
		Name:   "n",
		Scope:  "repo",
		URL:    "https://github.com/a/b",
		Labels: []string{"self-hosted"},
		Cache: &store.CacheConfig{
			Enabled:  true,
			Type:     "bind",
			HostPath: "/scratch/build-cache/n",
		},
	}
	env := strings.Join(m.buildEnv(rec, "", "", "/srv/gha-work/n"), "\n")
	if !strings.Contains(env, "RUNNER_WORKDIR=/srv/gha-work/n") {
		t.Fatal(env)
	}
	if !strings.Contains(env, "RUNNER_CACHE=/scratch/build-cache/n") {
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
