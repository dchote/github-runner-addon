package rest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/dchote/github-runner-addon/internal/runner"
	"github.com/dchote/github-runner-addon/internal/store"
	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
)

const maxCreateBodyBytes = 1 << 20 // 1 MiB

type Handlers struct {
	Manager *runner.Manager
	Docker  interface {
		Ping(ctx context.Context) error
	}
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	dockerOK := false
	var dockerErr string
	if h.Docker != nil {
		if err := h.Docker.Ping(r.Context()); err == nil {
			dockerOK = true
		} else {
			dockerErr = publicErr(err)
		}
	} else {
		dockerErr = "docker client unavailable"
	}

	storeOK := true
	var storeErr string
	if h.Manager != nil && h.Manager.Store != nil {
		if err := h.Manager.Store.Readable(); err != nil {
			storeOK = false
			storeErr = publicErr(err)
		}
	}

	counts := map[string]int{}
	if h.Manager != nil {
		if c, err := h.Manager.StatusCounts(r.Context()); err == nil {
			counts = c
		}
	}

	version := ""
	patConfigured := false
	mountSock := false
	image := ""
	orphans := []runner.OrphanView{}
	if h.Manager != nil {
		version = h.Manager.Version
		patConfigured = h.Manager.PATConfigured()
		mountSock = h.Manager.MountDockerSock
		image = h.Manager.Image
		orphans = h.Manager.Orphans()
	}

	WriteOK(w, http.StatusOK, map[string]interface{}{
		"status":  "ok",
		"version": version,
		"docker": map[string]interface{}{
			"available": dockerOK,
			"engine":    "Docker",
			"error":     dockerErr,
		},
		"store": map[string]interface{}{
			"readable": storeOK,
			"error":    storeErr,
		},
		"runners":               counts,
		"github_pat_configured": patConfigured,
		"runner_image":          image,
		"mount_docker_sock":     mountSock,
		"orphans":               orphans,
	})
}

func (h *Handlers) ListRunners(w http.ResponseWriter, r *http.Request) {
	list, err := h.Manager.List(r.Context())
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list runners", nil)
		slog.Error("list runners", "err", err)
		return
	}
	WriteOK(w, http.StatusOK, map[string]interface{}{"runners": list})
}

func (h *Handlers) GetRunner(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	v, err := h.Manager.Get(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "runner not found", nil)
		return
	}
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get runner", nil)
		slog.Error("get runner", "err", err)
		return
	}
	WriteOK(w, http.StatusOK, v)
}

func (h *Handlers) CreateRunner(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateBodyBytes)
	var req runner.CreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", nil)
		return
	}
	v, err := h.Manager.Create(r.Context(), req)
	if h.writeRunnerErr(w, err) {
		return
	}
	WriteOK(w, http.StatusCreated, v)
}

func (h *Handlers) PatchRunner(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateBodyBytes)
	id := chi.URLParam(r, "id")
	var req runner.PatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", nil)
		return
	}
	v, err := h.Manager.Patch(r.Context(), id, req)
	if h.writeRunnerErr(w, err) {
		return
	}
	WriteOK(w, http.StatusOK, v)
}

func (h *Handlers) RecreateRunner(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCreateBodyBytes)
	id := chi.URLParam(r, "id")
	var req runner.RecreateRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body", nil)
			return
		}
	}
	v, err := h.Manager.Recreate(r.Context(), id, req)
	if h.writeRunnerErr(w, err) {
		return
	}
	WriteOK(w, http.StatusOK, v)
}

func (h *Handlers) StartRunner(w http.ResponseWriter, r *http.Request) {
	h.applyLifecycle(w, r, h.Manager.Start)
}

func (h *Handlers) StopRunner(w http.ResponseWriter, r *http.Request) {
	h.applyLifecycle(w, r, h.Manager.Stop)
}

func (h *Handlers) RestartRunner(w http.ResponseWriter, r *http.Request) {
	h.applyLifecycle(w, r, h.Manager.Restart)
}

func (h *Handlers) applyLifecycle(w http.ResponseWriter, r *http.Request, fn func(context.Context, string) (runner.View, error)) {
	id := chi.URLParam(r, "id")
	v, err := fn(r.Context(), id)
	if h.writeRunnerErr(w, err) {
		return
	}
	WriteOK(w, http.StatusOK, v)
}

func (h *Handlers) DeleteRunner(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.Manager.Delete(r.Context(), id)
	if h.writeRunnerErr(w, err) {
		return
	}
	WriteOK(w, http.StatusOK, map[string]interface{}{"id": id, "deleted": true})
}

func (h *Handlers) RunnerLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	follow := r.URL.Query().Get("follow") == "1" || strings.EqualFold(r.URL.Query().Get("follow"), "true")
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "200"
	}
	if _, err := strconv.Atoi(tail); err != nil {
		tail = "200"
	}
	rc, err := h.Manager.Logs(r.Context(), id, follow, tail)
	if h.writeRunnerErr(w, err) {
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if follow {
		w.Header().Set("Cache-Control", "no-cache")
	}
	flusher, _ := w.(http.Flusher)
	reader := bufio.NewReader(rc)
	buf := make([]byte, 4096)
	for {
		n, readErr := reader.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr != nil {
			return
		}
	}
}

func (h *Handlers) writeRunnerErr(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		WriteError(w, http.StatusNotFound, "NOT_FOUND", "runner not found", nil)
	case errors.Is(err, store.ErrConflict):
		WriteError(w, http.StatusConflict, "CONFLICT", "runner name or container already exists", nil)
	case errors.Is(err, runner.ErrRunnerBusy):
		WriteError(w, http.StatusConflict, "RUNNER_BUSY", publicErr(err), nil)
	case errors.Is(err, runner.ErrValidation):
		WriteError(w, http.StatusBadRequest, "VALIDATION_ERROR", publicErr(err), nil)
	case errors.Is(err, runner.ErrGitHub):
		WriteError(w, http.StatusBadGateway, "GITHUB_ERROR", publicErr(err), nil)
	case errors.Is(err, runner.ErrRateLimited):
		WriteError(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many create/recreate requests; try again shortly", nil)
	case errors.Is(err, runner.ErrDockerUnavailable), isDockerUnavailable(err):
		WriteError(w, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker engine unavailable", nil)
	default:
		slog.Error("runner operation failed", "err", err)
		WriteError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "operation failed", nil)
	}
	return true
}

func publicErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Strip wrapped sentinel prefixes for cleaner client messages.
	for _, prefix := range []string{
		"validation error: ",
		"github api error: ",
		"rate limited: ",
		"docker unavailable: ",
		"runner busy: ",
	} {
		if strings.HasPrefix(msg, prefix) {
			msg = strings.TrimPrefix(msg, prefix)
			break
		}
	}
	if len(msg) > 500 {
		return msg[:500] + "…"
	}
	return msg
}

func isDockerUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if client.IsErrConnectionFailed(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cannot connect") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "docker unavailable") ||
		strings.Contains(msg, "is the docker daemon running")
}
