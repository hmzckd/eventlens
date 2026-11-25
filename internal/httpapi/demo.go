package httpapi

import (
	"bytes"
	"embed"
	"html/template"
	"net/http"
)

//go:embed web/demo.html
var webFiles embed.FS

var demoPage = template.Must(template.ParseFS(webFiles, "web/demo.html"))

func handleDemo(writeKey string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		if err := demoPage.Execute(&buf, map[string]string{"WriteKey": writeKey}); err != nil {
			http.Error(w, "demo page failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(buf.Bytes())
	}
}
