package rest

import (
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/dchote/github-runner-addon/internal/container/docker"
	"github.com/dchote/github-runner-addon/internal/runner"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(mgr *runner.Manager, dockerClient *docker.Client, feFS fs.FS) http.Handler {
	h := &Handlers{Manager: mgr}
	if dockerClient != nil {
		h.Docker = dockerClient
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	r.Get("/health", h.Health)
	r.Get("/docs", swaggerUIHandler())
	r.Get("/docs/openapi.yaml", openAPIHandler())
	r.Get("/docs/swagger-ui.css", swaggerAssetHandler("swagger-ui.css"))
	r.Get("/docs/swagger-ui-bundle.js", swaggerAssetHandler("swagger-ui-bundle.js"))
	r.Get("/ws", h.WebSocket)

	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/health", h.Health)
		api.Get("/openapi.yaml", openAPIHandler())
		api.With(middleware.Timeout(60*time.Second)).Get("/runners", h.ListRunners)
		api.With(middleware.Timeout(10*time.Minute)).Post("/runners", h.CreateRunner)
		api.With(middleware.Timeout(60*time.Second)).Get("/runners/{id}", h.GetRunner)
		api.With(middleware.Timeout(2*time.Minute)).Patch("/runners/{id}", h.PatchRunner)
		api.With(middleware.Timeout(2*time.Minute)).Delete("/runners/{id}", h.DeleteRunner)
		api.With(middleware.Timeout(60*time.Second)).Post("/runners/{id}/start", h.StartRunner)
		api.With(middleware.Timeout(60*time.Second)).Post("/runners/{id}/stop", h.StopRunner)
		api.With(middleware.Timeout(60*time.Second)).Post("/runners/{id}/restart", h.RestartRunner)
		api.With(middleware.Timeout(10*time.Minute)).Post("/runners/{id}/recreate", h.RecreateRunner)
		// No timeout: follow streams may run indefinitely.
		api.Get("/runners/{id}/logs", h.RunnerLogs)
	})

	if feFS != nil {
		spa := spaHandler(feFS)
		r.Get("/", spa)
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			if strings.HasPrefix(req.URL.Path, "/api/") ||
				strings.HasPrefix(req.URL.Path, "/docs") ||
				req.URL.Path == "/health" ||
				req.URL.Path == "/ws" {
				http.NotFound(w, req)
				return
			}
			spa(w, req)
		})
	}

	return r
}
