package http

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed dashboard
var dashboardFS embed.FS

func DashboardHandler() http.Handler {
	sub, _ := fs.Sub(dashboardFS, "dashboard")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimPrefix(r.URL.Path, "/")
		if filePath == "" {
			filePath = "index.html"
		}

		f, err := sub.Open(filePath)
		if err != nil {
			filePath = "index.html"
			f, err = sub.Open(filePath)
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		defer func() { _ = f.Close() }()

		switch path.Ext(filePath) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		}

		reader, ok := f.(io.Reader)
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = io.Copy(w, reader)
	})
}
