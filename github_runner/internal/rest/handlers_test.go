package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dchote/github-runner-addon/internal/runner"
	"github.com/dchote/github-runner-addon/internal/store"
)

func TestHealthHandler(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	mgr := runner.NewManager(st, nil, nil, "myoung34/github-runner:latest", false, "0.2.0-test")
	h := &Handlers{Manager: mgr}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	h.Health(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var env struct {
		Result string                 `json:"result"`
		Data   map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Result != "ok" {
		t.Fatalf("%v", env)
	}
	if env.Data["version"] != "0.2.0-test" {
		t.Fatalf("version %v", env.Data["version"])
	}
	if env.Data["github_pat_configured"] != false {
		t.Fatalf("pat %v", env.Data["github_pat_configured"])
	}
	if env.Data["status"] != "degraded" {
		t.Fatalf("docker unavailable should be degraded, status=%v", env.Data["status"])
	}
}

func TestCreateValidation(t *testing.T) {
	dir := t.TempDir()
	st := store.New(filepath.Join(dir, "runners.json"))
	mgr := runner.NewManager(st, nil, nil, "img", false, "0.2.0")
	h := &Handlers{Manager: mgr}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runners", nil)
	rr := httptest.NewRecorder()
	h.CreateRunner(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWriteRunnerErrImagePull(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	h.writeRunnerErr(rr, fmt.Errorf("%w: pull img: connection refused", runner.ErrImagePull))
	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "IMAGE_PULL_ERROR") {
		t.Fatalf("body=%s", rr.Body.String())
	}
}
