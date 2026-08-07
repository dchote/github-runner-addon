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

// resolveWorkVolumeName returns the auto-managed per-runner work volume name.
func resolveWorkVolumeName(rec store.Runner) string {
	if rec.ContainerName == "" {
		return ""
	}
	return rec.ContainerName + "-work"
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
	out.Target = cacheTarget(&out)
	if out.Type == "bind" {
		out.VolumeName = ""
	} else {
		out.HostPath = ""
	}
	return &out
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
		if err := validateBindHostPath(c.HostPath); err != nil {
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

func validateBindHostPath(hostPath string) error {
	hostPath = strings.TrimSpace(hostPath)
	if hostPath == "" {
		return fmt.Errorf("%w: cache.host_path is required when cache.type is bind", ErrValidation)
	}
	if err := validateMountPath(hostPath, "cache.host_path"); err != nil {
		return err
	}
	if strings.Contains(hostPath, "\x00") {
		return fmt.Errorf("%w: invalid cache.host_path", ErrValidation)
	}
	return nil
}

// buildExtraMounts builds cache mounts plus the job workdir same-path bind at workdirBind
// (the Docker volume Mountpoint).
func buildExtraMounts(rec store.Runner, workdirBind string) []mount.Mount {
	var mounts []mount.Mount
	if rec.Cache != nil && rec.Cache.Enabled {
		target := cacheTarget(rec.Cache)
		if cacheType(rec.Cache) == "bind" {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeBind,
				Source:   strings.TrimSpace(rec.Cache.HostPath),
				Target:   target,
				ReadOnly: rec.Cache.ReadOnly,
			})
		} else {
			mounts = append(mounts, mount.Mount{
				Type:     mount.TypeVolume,
				Source:   resolveCacheVolumeName(rec),
				Target:   target,
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
