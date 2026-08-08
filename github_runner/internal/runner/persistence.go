package runner

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/docker/docker/api/types/mount"

	"github.com/dchote/github-runner-addon/internal/store"
)

const (
	defaultCacheTarget = "/cache"

	// StopTimeoutSecs is the container Config.StopTimeout for managed runners.
	StopTimeoutSecs = 120
)

var (
	volumeNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

	// Forbidden as bind host sources and as container mount targets.
	forbiddenPaths = map[string]struct{}{
		"/":                    {},
		"/etc":                 {},
		"/proc":                {},
		"/sys":                 {},
		"/var/run/docker.sock": {},
	}
)

func cacheType(c *store.CacheConfig) string {
	if c == nil {
		return ""
	}
	typ := strings.ToLower(strings.TrimSpace(c.Type))
	if typ == "" {
		return "volume"
	}
	return typ
}

func cacheTarget(c *store.CacheConfig) string {
	if c == nil {
		return defaultCacheTarget
	}
	t := strings.TrimSpace(c.Target)
	if t == "" {
		return defaultCacheTarget
	}
	return t
}

// resolveCacheVolumeName returns the Docker volume name for a volume-mode cache, or "".
func resolveCacheVolumeName(rec store.Runner) string {
	if rec.Cache == nil || !rec.Cache.Enabled {
		return ""
	}
	if cacheType(rec.Cache) != "volume" {
		return ""
	}
	if name := strings.TrimSpace(rec.Cache.VolumeName); name != "" {
		return name
	}
	return rec.ContainerName + "-cache"
}

// cacheVolumeOwned reports whether a failed-create rollback may remove the cache volume
// (auto-named volumes only; never shared/custom names or binds).
func cacheVolumeOwned(rec store.Runner) bool {
	if rec.Cache == nil || !rec.Cache.Enabled {
		return false
	}
	if cacheType(rec.Cache) != "volume" {
		return false
	}
	return strings.TrimSpace(rec.Cache.VolumeName) == ""
}

func normalizeCache(c *store.CacheConfig) *store.CacheConfig {
	if c == nil || !c.Enabled {
		return nil
	}
	out := *c
	out.Enabled = true
	out.Type = cacheType(&out)
	out.VolumeName = strings.TrimSpace(out.VolumeName)
	out.HostPath = strings.TrimSpace(out.HostPath)
	if out.Type == "bind" {
		out.VolumeName = ""
		// Same-path rule: bind mounts always use host_path as the container target.
		if hp := path.Clean(out.HostPath); hp != "" && hp != "." {
			out.HostPath = hp
			out.Target = hp
		} else {
			out.Target = cacheTarget(&out)
		}
	} else {
		out.HostPath = ""
		out.Target = cacheTarget(&out)
	}
	return &out
}

// resolveCacheEffective returns the absolute path workflows should use (RUNNER_CACHE).
func resolveCacheEffective(c *store.CacheConfig) string {
	if c == nil || !c.Enabled {
		return ""
	}
	if cacheType(c) == "bind" {
		return path.Clean(strings.TrimSpace(c.HostPath))
	}
	return path.Clean(cacheTarget(c))
}

func validateCache(c *store.CacheConfig) error {
	if c == nil || !c.Enabled {
		return nil
	}
	typ := cacheType(c)
	if typ != "volume" && typ != "bind" {
		return fmt.Errorf("%w: cache.type must be volume or bind", ErrValidation)
	}
	target := cacheTarget(c)
	if err := validateMountPath(target, "cache.target"); err != nil {
		return err
	}
	if target == configFilesDir {
		return fmt.Errorf("%w: cache.target must not be %s", ErrValidation, configFilesDir)
	}
	if typ == "bind" {
		if err := validateHostBindPath(c.HostPath, "cache.host_path"); err != nil {
			return err
		}
	} else {
		name := strings.TrimSpace(c.VolumeName)
		if name != "" && !volumeNameRe.MatchString(name) {
			return fmt.Errorf("%w: invalid cache.volume_name", ErrValidation)
		}
	}
	return nil
}

// Soft advisory copy (keep in sync with frontend cacheSiblingPathWarning).
const cacheVolumeSiblingFmt = "cache uses a named volume mounted at %q; sibling Docker and Buildx type=local that bind-mount that path on the Docker host will not see this volume. Prefer a host bind (same-path) and use $RUNNER_CACHE in workflows when sibling visibility is required."

// cacheSiblingWarnings returns soft operator warnings for cache mounts that will
// confuse sibling Docker / Buildx. Bind mounts are always same-path after normalize.
// Never fails validation — named volumes remain allowed.
func cacheSiblingWarnings(c *store.CacheConfig) []string {
	if c == nil || !c.Enabled {
		return nil
	}
	if cacheType(c) != "volume" {
		return nil
	}
	target := path.Clean(cacheTarget(c))
	return []string{fmt.Sprintf(cacheVolumeSiblingFmt, target)}
}

func validateMountPath(p, label string) error {
	p = strings.TrimSpace(p)
	if p == "" || !strings.HasPrefix(p, "/") || p == "/" {
		return fmt.Errorf("%w: %s must be an absolute path", ErrValidation, label)
	}
	if strings.Contains(p, "//") || path.Clean(p) != p {
		return fmt.Errorf("%w: invalid %s", ErrValidation, label)
	}
	for _, part := range strings.Split(p, "/") {
		if part == ".." || part == "." {
			return fmt.Errorf("%w: invalid %s", ErrValidation, label)
		}
	}
	if _, bad := forbiddenPaths[path.Clean(p)]; bad {
		return fmt.Errorf("%w: %s %q is not allowed", ErrValidation, label, p)
	}
	return nil
}

// buildExtraMounts builds cache mounts plus the job workdir same-path host bind.
func buildExtraMounts(rec store.Runner, workdirBind string) []mount.Mount {
	var mounts []mount.Mount
	if rec.Cache != nil && rec.Cache.Enabled {
		if cacheType(rec.Cache) == "bind" {
			hp := path.Clean(strings.TrimSpace(rec.Cache.HostPath))
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   hp,
				Target:   hp,
				ReadOnly: rec.Cache.ReadOnly,
			})
		} else {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeVolume,
				Source:   resolveCacheVolumeName(rec),
				Target:   cacheTarget(rec.Cache),
				ReadOnly: rec.Cache.ReadOnly,
			})
		}
	}
	if hp := strings.TrimSpace(workdirBind); hp != "" {
		mounts = append(mounts, mount.Mount{
			Type:   mount.TypeBind,
			Source: hp,
			Target: hp,
		})
	}
	return mounts
}

func cacheVolumeRefs(runners []store.Runner, volumeName string) int {
	if volumeName == "" {
		return 0
	}
	n := 0
	for _, r := range runners {
		if resolveCacheVolumeName(r) == volumeName {
			n++
		}
	}
	return n
}
