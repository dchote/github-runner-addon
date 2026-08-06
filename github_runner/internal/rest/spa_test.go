package rest

import (
	"net/http"
	"testing"
)

func TestAppBaseHref(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	if got := appBaseHref(r); got != "/" {
		t.Fatalf("default base: %q", got)
	}

	r.Header.Set("X-Ingress-Path", "/api/hassio_ingress/abc123")
	if got := appBaseHref(r); got != "/api/hassio_ingress/abc123/" {
		t.Fatalf("ingress header: %q", got)
	}

	r.Header.Set("X-Ingress-Path", "//evil.example/x")
	if got := appBaseHref(r); got != "/" {
		t.Fatalf("reject unsafe ingress path: %q", got)
	}

	r2, _ := http.NewRequest(http.MethodGet, "/api/hassio_ingress/tok/runners", nil)
	if got := appBaseHref(r2); got != "/" {
		t.Fatalf("path alone is not trusted without header: %q", got)
	}
}

func TestCheckWSOrigin(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "http://localhost:8099/ws", nil)
	r.Host = "localhost:8099"
	if !checkWSOrigin(r) {
		t.Fatal("empty origin should allow")
	}
	r.Header.Set("Origin", "http://localhost:8099")
	if !checkWSOrigin(r) {
		t.Fatal("same origin should allow")
	}
	r.Header.Set("Origin", "http://evil.example")
	if checkWSOrigin(r) {
		t.Fatal("cross origin should deny")
	}

	r.Header.Set("Origin", "http://homeassistant.local:8123")
	r.Host = "172.30.32.1:8099"
	r.Header.Set("X-Forwarded-Host", "homeassistant.local:8123")
	if !checkWSOrigin(r) {
		t.Fatal("X-Forwarded-Host should allow HA UI origin")
	}

	r.Header.Del("X-Forwarded-Host")
	r.Header.Set("X-Ingress-Path", "/api/hassio_ingress/tok")
	if !checkWSOrigin(r) {
		t.Fatal("X-Ingress-Path should allow under HA ingress")
	}
}
