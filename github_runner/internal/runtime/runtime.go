package runtime

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultHTTPPort = "8099"
	DefaultDataDir  = "./data"
	DefaultVersion  = "0.5.0"
)

// Config holds process configuration.
type Config struct {
	DataDir         string
	HTTPPort        string
	ListenAddr      string
	FrontendEmbed   bool
	LogLevel        string
	RunnerImage     string
	MountDockerSock bool
	GitHubPAT       string
	AppVersion      string
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
		MountDockerSock: envOr("MOUNT_DOCKER_SOCK", "false") == "true",
		GitHubPAT:       strings.TrimSpace(os.Getenv("GITHUB_PAT")),
		AppVersion:      envOr("APP_VERSION", DefaultVersion),
	}
}

func (c Config) Addr() string {
	if c.ListenAddr != "" {
		return c.ListenAddr
	}
	return ":" + c.HTTPPort
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
