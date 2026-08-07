package runner

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/dchote/github-runner-addon/internal/store"
)

const (
	defaultWorkdirRoot = "/srv/gha-work"
	runnerConfigFile   = ".runner"
)

// defaultWorkdirHostPath is the per-runner same-path host bind when workdir_host_path is unset.
func defaultWorkdirHostPath(rec store.Runner) string {
	suffix := strings.TrimPrefix(rec.ContainerName, "gha-runner-")
	if suffix == "" || suffix == rec.ContainerName {
		if n := strings.TrimSpace(rec.Name); n != "" {
			suffix = n
		} else {
			suffix = "runner"
		}
	}
	return path.Join(defaultWorkdirRoot, suffix)
}

// resolveWorkdirHostPath returns the configured or default host workdir path.
func resolveWorkdirHostPath(rec store.Runner) string {
	if p := strings.TrimSpace(rec.WorkdirHostPath); p != "" {
		return p
	}
	return defaultWorkdirHostPath(rec)
}

// normalizeWorkdirHostPath stores empty when the value matches the default (keeps runners.json tidy).
func normalizeWorkdirHostPath(rec store.Runner, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == defaultWorkdirHostPath(rec) {
		return ""
	}
	return raw
}

func validateWorkdirHostPath(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return nil
	}
	if err := validateHostBindPath(p, "workdir_host_path"); err != nil {
		return err
	}
	clean := path.Clean(p)
	if clean == configFilesDir || strings.HasPrefix(clean+"/", configFilesDir+"/") {
		return fmt.Errorf("%w: workdir_host_path must not overlap %s", ErrValidation, configFilesDir)
	}
	// Docker volume Mountpoints are not a valid sibling-Docker recipe.
	if clean == "/var/lib/docker" || strings.HasPrefix(clean+"/", "/var/lib/docker/") {
		return fmt.Errorf("%w: workdir_host_path must not be under /var/lib/docker — use a real host directory (e.g. %s/<name>)", ErrValidation, defaultWorkdirRoot)
	}
	return nil
}

func validateHostBindPath(hostPath, label string) error {
	hostPath = strings.TrimSpace(hostPath)
	if hostPath == "" {
		return fmt.Errorf("%w: %s is required", ErrValidation, label)
	}
	if err := validateMountPath(hostPath, label); err != nil {
		return err
	}
	if strings.Contains(hostPath, "\x00") {
		return fmt.Errorf("%w: invalid %s", ErrValidation, label)
	}
	return nil
}

// legacyWorkVolumeName is the obsolete Mountpoint-based work volume from 0.3.1–0.3.3.
func legacyWorkVolumeName(rec store.Runner) string {
	if rec.ContainerName == "" {
		return ""
	}
	return rec.ContainerName + "-work"
}

type runnerDotConfig struct {
	WorkFolder string `json:"workFolder"`
}

func parseRunnerWorkFolder(raw []byte) (string, error) {
	var cfg runnerDotConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}
	return strings.TrimSpace(cfg.WorkFolder), nil
}
