package runtime

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultHTTPPort = "8099"
	DefaultDataDir  = "./data"
	DefaultVersion  = "0.7.1"
)

// Config holds process configuration.
type Config struct {
	DataDir              string
	HTTPPort             string
	ListenAddr           string
	FrontendEmbed        bool
	LogLevel             string
	RunnerImage          string
	MountDockerSock      bool
	GitHubPAT            string
	AppVersion           string
	ImageRefreshInterval time.Duration
}

// LoadFromEnv builds Config from environment and defaults.
func LoadFromEnv() Config {
	dataDir := envOr("DATA_DIR", DefaultDataDir)
	if _, err := os.Stat("/data"); err == nil && os.Getenv("DATA_DIR") == "" {
		// Home Assistant addon convention
		dataDir = "/data"
	}
	port := envOr("HTTP_PORT", DefaultHTTPPort)
	return Config{
		DataDir:       dataDir,
		HTTPPort:      port,
		ListenAddr:    envOr("LISTEN_ADDR", ""),
		FrontendEmbed: true,
		LogLevel:      envOr("LOG_LEVEL", "info"),
		RunnerImage:   envOr("RUNNER_IMAGE", "myoung34/github-runner:latest"), // keep in sync with runner.DefaultRunnerImage
		// Opt-in: mounting the host Docker socket into runners is powerful and risky.
		MountDockerSock:      envOr("MOUNT_DOCKER_SOCK", "false") == "true",
		GitHubPAT:            strings.TrimSpace(os.Getenv("GITHUB_PAT")),
		AppVersion:           envOr("APP_VERSION", DefaultVersion),
		ImageRefreshInterval: ParseImageRefreshDays(os.Getenv("RUNNER_IMAGE_REFRESH_DAYS")),
	}
}

// ParseImageRefreshDays reads the addon option in days. Empty or invalid values
// use 7 (one week). Zero disables automatic image checks. Values above 365 are capped.
func ParseImageRefreshDays(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 7 * 24 * time.Hour
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 7 * 24 * time.Hour
	}
	if n == 0 {
		return 0
	}
	if n > 365 {
		n = 365
	}
	return time.Duration(n) * 24 * time.Hour
}

func (c Config) Addr() string {
	if c.ListenAddr != "" {
		return c.ListenAddr
	}
	// Inside Docker / Home Assistant the process must listen on all interfaces so
	// ingress and published ports can connect. On a host binary, bind loopback.
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return ":" + c.HTTPPort
	}
	if os.Getenv("SUPERVISOR_TOKEN") != "" {
		return ":" + c.HTTPPort
	}
	return "127.0.0.1:" + c.HTTPPort
}

func (c Config) EnsureDataDir() error {
	return os.MkdirAll(c.DataDir, 0o755)
}

func (c Config) RunnersPath() string {
	return filepath.Join(c.DataDir, "runners.json")
}

func ParseLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug", "info", "warn", "warning", "error":
		if strings.EqualFold(level, "warning") {
			return "warn"
		}
		return strings.ToLower(level)
	default:
		return "info"
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
