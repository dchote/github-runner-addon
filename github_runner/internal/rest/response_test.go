package rest

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnceHeaderWriterSuppressesDuplicate(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &onceHeaderWriter{ResponseWriter: rec}
	w.WriteHeader(http.StatusGatewayTimeout)
	w.WriteHeader(http.StatusOK)
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("code=%d", rec.Code)
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("write changed code to %d", rec.Code)
	}
}

type wrapRW struct {
	http.ResponseWriter
}

func (w *wrapRW) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func TestHeaderWrittenUnwraps(t *testing.T) {
	rec := httptest.NewRecorder()
	inner := &onceHeaderWriter{ResponseWriter: rec}
	outer := &wrapRW{ResponseWriter: inner}
	if headerWritten(outer) {
		t.Fatal("expected false before WriteHeader")
	}
	inner.WriteHeader(http.StatusGatewayTimeout)
	if !headerWritten(outer) {
		t.Fatal("expected true via Unwrap")
	}
	WriteOK(outer, http.StatusOK, map[string]string{"x": "1"})
	if rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("WriteOK should no-op after timeout, code=%d", rec.Code)
	}
}
