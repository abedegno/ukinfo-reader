package main

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
)

// Each HTML response references the exact embedded asset revision. This prevents
// a browser/CDN from mixing new markup with scripts cached from an older build.
func webHandler(ui fs.FS) (http.Handler, error) {
	index, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		return nil, err
	}
	revisions := map[string]string{}
	err = fs.WalkDir(ui, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !(strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css")) {
			return nil
		}
		data, readErr := fs.ReadFile(ui, path)
		if readErr != nil {
			return readErr
		}
		revisions["/"+path] = fmt.Sprintf("%x", sha256.Sum256(data))[:16]
		return nil
	})
	if err != nil {
		return nil, err
	}
	html := string(index)
	for path, revision := range revisions {
		html = strings.ReplaceAll(html, `"`+path+`"`, `"`+path+`?v=`+revision+`"`)
	}
	files := http.FileServer(http.FS(ui))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if r.Method != http.MethodHead {
				_, _ = w.Write([]byte(html))
			}
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		if revision, ok := revisions[r.URL.Path]; ok && r.URL.Query().Get("v") == revision {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	}), nil
}
