package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestParseProjectURL(t *testing.T) {
	cases := []struct {
		raw     string
		scope   string
		owner   string
		repo    string
		apiBase string
		wantErr bool
	}{
		{"https://github.com/acme/app", "repo", "acme", "app", "https://api.github.com", false},
		{"https://github.com/acme", "org", "acme", "", "https://api.github.com", false},
		{"https://ghes.example.com/acme/app", "repo", "acme", "app", "https://ghes.example.com/api/v3", false},
		{"https://github.com/a/b/c", "", "", "", "", true},
		{"not-a-url", "", "", "", "", true},
	}
	for _, tc := range cases {
		info, err := ParseProjectURL(tc.raw)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error", tc.raw)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", tc.raw, err)
		}
		if info.Scope != tc.scope || info.Owner != tc.owner || info.Repo != tc.repo || info.APIBase != tc.apiBase {
			t.Fatalf("%s: got %+v", tc.raw, info)
		}
	}
}

func TestMintAndDeregister(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/app/actions/runners/registration-token", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-pat" {
			t.Fatalf("auth header")
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "REGTOK"})
	})
	mux.HandleFunc("/repos/acme/app/actions/runners", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runners": []map[string]any{{"id": 42, "name": "lab-1"}},
		})
	})
	mux.HandleFunc("/repos/acme/app/actions/runners/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New("test-pat")
	c.HTTPClient = &http.Client{Transport: &hostRewrite{target: srv.URL}}

	tok, err := c.MintRegistrationToken(context.Background(), "https://github.com/acme/app")
	if err != nil || tok != "REGTOK" {
		t.Fatalf("mint: tok=%q err=%v", tok, err)
	}
	if err := c.DeregisterRunner(context.Background(), "https://github.com/acme/app", "lab-1"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
}

type hostRewrite struct {
	target string
}

func (h *hostRewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	u, err := url.Parse(h.target)
	if err != nil {
		return nil, err
	}
	req.URL.Scheme = u.Scheme
	req.URL.Host = u.Host
	req.Host = u.Host
	return http.DefaultTransport.RoundTrip(req)
}
