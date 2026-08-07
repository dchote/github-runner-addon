package runner

import (
	"strings"
	"testing"

	"github.com/docker/docker/api/types/mount"

	"github.com/dchote/github-runner-addon/internal/store"
)

func TestValidateCacheVolume(t *testing.T) {
	c := &store.CacheConfig{Enabled: true, Type: "volume", Target: "/cache"}
	if err := validateCache(c, false, ""); err != nil {
		t.Fatal(err)
	}
	c.VolumeName = "bad name!"
	if err := validateCache(c, false, ""); err == nil {
		t.Fatal("expected invalid volume name")
	}
}

func TestValidateCacheBind(t *testing.T) {
	c := &store.CacheConfig{Enabled: true, Type: "bind", HostPath: "/srv/runner-cache", Target: "/cache"}
	if err := validateCache(c, true, ""); err != nil {
		t.Fatal(err)
	}
	c.HostPath = ""
	if err := validateCache(c, false, ""); err == nil {
		t.Fatal("expected host_path required")
	}
	c.HostPath = "/etc"
	if err := validateCache(c, false, ""); err == nil {
		t.Fatal("expected forbidden path")
	}
	c.HostPath = "/srv/../etc"
	if err := validateCache(c, false, ""); err == nil {
		t.Fatal("expected .. rejected")
	}
}

func TestValidateCacheForbiddenTargets(t *testing.T) {
	for _, target := range []string{"/etc", "/proc", "/sys", "/var/run/docker.sock", "/"} {
		c := &store.CacheConfig{Enabled: true, Type: "volume", Target: target}
		if err := validateCache(c, false, ""); err == nil {
			t.Fatalf("expected forbidden target %q", target)
		}
	}
}

func TestValidateCacheCollisions(t *testing.T) {
	c := &store.CacheConfig{Enabled: true, Type: "volume", Target: configFilesDir}
	if err := validateCache(c, false, ""); err == nil {
		t.Fatal("expected collision with registration dir")
	}
	c.Target = workdirPath
	if err := validateCache(c, true, ""); err == nil {
		t.Fatal("expected collision with workdir")
	}
	c.Target = "/srv/gha-work/builder"
	if err := validateCache(c, false, "/srv/gha-work/builder"); err == nil {
		t.Fatal("expected collision with workdir_host_path")
	}
}

func TestValidateWorkdirHostPath(t *testing.T) {
	if err := validateWorkdirHostPath(""); err != nil {
		t.Fatal(err)
	}
	if err := validateWorkdirHostPath("/srv/gha-work/supervisor-builder"); err != nil {
		t.Fatal(err)
	}
	if err := validateWorkdirHostPath("/work"); err == nil {
		t.Fatal("expected /work rejected")
	}
	if err := validateWorkdirHostPath("/tmp/runner/work"); err == nil {
		t.Fatal("expected ephemeral path rejected")
	}
	if err := validateWorkdirHostPath("/etc"); err == nil {
		t.Fatal("expected forbidden")
	}
}

func TestNormalizeCacheDisabled(t *testing.T) {
	if normalizeCache(nil) != nil {
		t.Fatal("nil")
	}
	if normalizeCache(&store.CacheConfig{Enabled: false}) != nil {
		t.Fatal("disabled")
	}
	got := normalizeCache(&store.CacheConfig{Enabled: true, HostPath: "/ignored", Type: "volume"})
	if got.Type != "volume" || got.Target != "/cache" || got.HostPath != "" {
		t.Fatalf("%+v", got)
	}
	got = normalizeCache(&store.CacheConfig{Enabled: true, Type: "bind", HostPath: "/srv/c", VolumeName: "x"})
	if got.VolumeName != "" || got.HostPath != "/srv/c" {
		t.Fatalf("%+v", got)
	}
}

func TestResolveNamesAndOwnership(t *testing.T) {
	rec := store.Runner{
		ContainerName:  "gha-runner-lab",
		Cache:          &store.CacheConfig{Enabled: true, Type: "volume"},
		PersistWorkdir: true,
	}
	if got := resolveCacheVolumeName(rec); got != "gha-runner-lab-cache" {
		t.Fatalf("cache vol: %s", got)
	}
	if got := resolveWorkVolumeName(rec); got != "gha-runner-lab-work" {
		t.Fatalf("work vol: %s", got)
	}
	if !cacheVolumeOwned(rec) {
		t.Fatal("auto-named should be owned")
	}
	rec.Cache.VolumeName = "shared-cache"
	if resolveCacheVolumeName(rec) != "shared-cache" {
		t.Fatal("custom name")
	}
	if cacheVolumeOwned(rec) {
		t.Fatal("custom name must not be owned for rollback")
	}
	rec.Cache.Type = "bind"
	rec.Cache.HostPath = "/srv/cache"
	if resolveCacheVolumeName(rec) != "" {
		t.Fatal("bind has no volume")
	}
	if cacheVolumeOwned(rec) {
		t.Fatal("bind not owned")
	}
	rec.WorkdirHostPath = "/srv/gha-work/lab"
	if resolveWorkVolumeName(rec) != "" {
		t.Fatal("host bind workdir has no volume")
	}
	if resolveRunnerWorkdir(rec) != "/srv/gha-work/lab" {
		t.Fatal(resolveRunnerWorkdir(rec))
	}
}

func TestBuildExtraMounts(t *testing.T) {
	rec := store.Runner{
		ContainerName:  "gha-runner-lab",
		PersistWorkdir: true,
		Cache: &store.CacheConfig{
			Enabled:  true,
			Type:     "bind",
			HostPath: "/srv/runner-cache",
			Target:   "/cache",
			ReadOnly: true,
		},
	}
	mounts := buildExtraMounts(rec)
	if len(mounts) != 2 {
		t.Fatalf("len=%d", len(mounts))
	}
	if mounts[0].Type != mount.TypeBind || mounts[0].Source != "/srv/runner-cache" || mounts[0].Target != "/cache" || !mounts[0].ReadOnly {
		t.Fatalf("cache mount: %+v", mounts[0])
	}
	if mounts[1].Type != mount.TypeVolume || mounts[1].Source != "gha-runner-lab-work" || mounts[1].Target != "/work" {
		t.Fatalf("work mount: %+v", mounts[1])
	}

	rec.WorkdirHostPath = "/srv/gha-work/lab"
	mounts = buildExtraMounts(rec)
	if len(mounts) != 2 {
		t.Fatalf("bind workdir len=%d", len(mounts))
	}
	if mounts[1].Type != mount.TypeBind || mounts[1].Source != "/srv/gha-work/lab" || mounts[1].Target != "/srv/gha-work/lab" {
		t.Fatalf("same-path work mount: %+v", mounts[1])
	}
}

func TestCacheVolumeRefs(t *testing.T) {
	runners := []store.Runner{
		{ContainerName: "gha-runner-a", Cache: &store.CacheConfig{Enabled: true, Type: "volume", VolumeName: "shared"}},
		{ContainerName: "gha-runner-b", Cache: &store.CacheConfig{Enabled: true, Type: "volume", VolumeName: "shared"}},
		{ContainerName: "gha-runner-c", Cache: &store.CacheConfig{Enabled: true, Type: "volume"}},
	}
	if n := cacheVolumeRefs(runners, "shared"); n != 2 {
		t.Fatalf("shared refs=%d", n)
	}
	if n := cacheVolumeRefs(runners, "gha-runner-c-cache"); n != 1 {
		t.Fatalf("auto refs=%d", n)
	}
}

func TestStopTimeoutSecs(t *testing.T) {
	if stopTimeoutSecs(store.Runner{}) != StopTimeoutDefault {
		t.Fatal("default")
	}
	if stopTimeoutSecs(store.Runner{PersistWorkdir: true}) != StopTimeoutLong {
		t.Fatal("workdir")
	}
	if stopTimeoutSecs(store.Runner{WorkdirHostPath: "/srv/gha-work/x"}) != StopTimeoutLong {
		t.Fatal("workdir host bind")
	}
	if stopTimeoutSecs(store.Runner{Cache: &store.CacheConfig{Enabled: true}}) != StopTimeoutLong {
		t.Fatal("cache")
	}
}

func TestBuildEnvWorkdir(t *testing.T) {
	m := &Manager{}
	env := m.buildEnv(store.Runner{Name: "n", Scope: "repo", URL: "https://github.com/a/b", Labels: []string{"self-hosted"}}, "", "")
	joined := strings.Join(env, "\n")
	if !strings.Contains(joined, "RUNNER_WORKDIR=/tmp/runner/work") {
		t.Fatal(joined)
	}
	env = m.buildEnv(store.Runner{Name: "n", Scope: "repo", URL: "https://github.com/a/b", Labels: []string{"self-hosted"}, PersistWorkdir: true}, "", "")
	joined = strings.Join(env, "\n")
	if !strings.Contains(joined, "RUNNER_WORKDIR=/work") {
		t.Fatal(joined)
	}
	env = m.buildEnv(store.Runner{
		Name: "n", Scope: "repo", URL: "https://github.com/a/b", Labels: []string{"self-hosted"},
		PersistWorkdir: true, WorkdirHostPath: "/srv/gha-work/builder",
	}, "", "")
	joined = strings.Join(env, "\n")
	if !strings.Contains(joined, "RUNNER_WORKDIR=/srv/gha-work/builder") {
		t.Fatal(joined)
	}
}
