package rest

import (
	"bytes"
	"html"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
)

var safeIngressPath = regexp.MustCompile(`^/api/hassio_ingress/[A-Za-z0-9._~-]+$`)

func spaHandler(feFS fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if feFS == nil {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		_, statErr := fs.Stat(feFS, path)
		if statErr != nil {
			if isAssetPath(path) {
				http.NotFound(w, r)
				return
			}
			path = "index.html"
		}
		f, err := feFS.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, r)
			return
		}
		if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		if path == "index.html" {
			body, err := io.ReadAll(f)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			base := html.EscapeString(appBaseHref(r))
			baseTag := []byte("<base href=\"" + base + "\">")
			head := []byte("<head>")
			idx := bytes.Index(body, head)
			if idx >= 0 {
				body = bytes.Join([][]byte{body[:idx+len(head)], baseTag, body[idx+len(head):]}, nil)
			}
			_, _ = w.Write(body)
			return
		}
		if rs, ok := f.(io.ReadSeeker); ok {
			http.ServeContent(w, r, path, stat.ModTime(), rs)
		} else {
			_, _ = io.Copy(w, f)
		}
	}
}

func isAssetPath(path string) bool {
	return strings.HasPrefix(path, "assets/") ||
		strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".map") ||
		strings.HasSuffix(path, ".ico") ||
		strings.HasSuffix(path, ".png") ||
		strings.HasSuffix(path, ".svg") ||
		strings.HasSuffix(path, ".woff") ||
		strings.HasSuffix(path, ".woff2")
}

// appBaseHref returns the browser-facing base path for the SPA.
// Home Assistant ingress sets X-Ingress-Path (e.g. /api/hassio_ingress/<token>).
func appBaseHref(r *http.Request) string {
	if p := strings.TrimSpace(r.Header.Get("X-Ingress-Path")); p != "" {
		p = strings.TrimRight(p, "/")
		if safeIngressPath.MatchString(p) {
			return p + "/"
		}
		return "/"
	}
	return "/"
}
