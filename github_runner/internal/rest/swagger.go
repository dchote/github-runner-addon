package rest

import (
	"embed"
	"encoding/json"
	"html"
	"io/fs"
	"net/http"
	"strings"

	"github.com/dchote/github-runner-addon/api"
)

//go:embed swaggerui/*
var swaggerUIFS embed.FS

func swaggerUIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		base := appBaseHref(r)
		prefix := strings.TrimRight(base, "/")
		specPath := "/docs/openapi.yaml"
		cssPath := "/docs/swagger-ui.css"
		jsPath := "/docs/swagger-ui-bundle.js"
		if prefix != "" && prefix != "/" {
			specPath = prefix + specPath
			cssPath = prefix + cssPath
			jsPath = prefix + jsPath
		}
		specJSON, _ := json.Marshal(specPath)
		htmlBody := `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8" />
  <title>GitHub Runner Manager API</title>
  <base href="` + html.EscapeString(base) + `">
  <style>body{margin:0;font-family:system-ui,sans-serif}</style>
  <link rel="stylesheet" href="` + html.EscapeString(cssPath) + `" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="` + html.EscapeString(jsPath) + `"></script>
  <script>
    SwaggerUIBundle({ url: ` + string(specJSON) + `, dom_id: '#swagger-ui' });
  </script>
</body>
</html>`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(htmlBody))
	}
}

func swaggerAssetHandler(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(swaggerUIFS, "swaggerui/"+name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.HasSuffix(name, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(name, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "application/octet-stream")
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(b)
	}
}

func openAPIHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		_, _ = w.Write(api.OpenAPI)
	}
}
