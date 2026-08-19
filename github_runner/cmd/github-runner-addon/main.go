package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	ghclient "github.com/dchote/github-runner-addon/internal/github"
	"github.com/dchote/github-runner-addon/internal/rest"
	"github.com/dchote/github-runner-addon/internal/runner"
	"github.com/dchote/github-runner-addon/internal/runtime"
	"github.com/dchote/github-runner-addon/internal/store"
)

func main() {
	cfg := runtime.LoadFromEnv()
	frontendEmbed := flag.Bool("frontend-embed", cfg.FrontendEmbed, "serve embedded SPA when built with embed_frontend")
	httpPort := flag.String("http-port", cfg.HTTPPort, "HTTP listen port")
	dataDir := flag.String("data-dir", cfg.DataDir, "data directory for runners.json")
	flag.Parse()

	cfg.HTTPPort = *httpPort
	cfg.DataDir = *dataDir
	cfg.FrontendEmbed = *frontendEmbed

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: parseSlogLevel(runtime.ParseLogLevel(cfg.LogLevel)),
	})))

	if err := cfg.EnsureDataDir(); err != nil {
		slog.Error("data dir", "err", err)
		os.Exit(1)
	}

	st := store.New(cfg.RunnersPath())
	dockerClient, err := docker.New()
	if err != nil {
		slog.Warn("docker client unavailable", "err", err)
		dockerClient = nil
	} else {
		defer dockerClient.Close()
		if err := dockerClient.Ping(context.Background()); err != nil {
			slog.Warn("docker ping failed", "err", err)
		} else {
			slog.Info("docker available")
		}
	}

	var gh *ghclient.Client
	if cfg.GitHubPAT != "" {
		gh = ghclient.New(cfg.GitHubPAT)
		slog.Info("github PAT configured")
	}

	mgr := runner.NewManager(st, dockerClient, gh, cfg.RunnerImage, cfg.MountDockerSock, cfg.AppVersion)
	mgr.SetDockerFactory(docker.New)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.StartReconcileLoop(ctx, 2*time.Minute)

	fsys, err := getFrontendFS()
	if err != nil {
		slog.Warn("frontend fs", "err", err)
		fsys = nil
	}
	if !cfg.FrontendEmbed {
		fsys = nil
	}
	if fsys != nil {
		slog.Info("serving embedded frontend")
	} else {
		slog.Info("embedded frontend disabled; use Vite proxy for UI")
	}

	handler := rest.NewRouter(mgr, dockerClient, fsys)
	srv := &http.Server{
		Addr:              cfg.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("listening", "addr", cfg.Addr(), "mount_docker_sock", cfg.MountDockerSock, "version", cfg.AppVersion)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server", "err", err)
			os.Exit(1)
		}
	}()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
}

func parseSlogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
