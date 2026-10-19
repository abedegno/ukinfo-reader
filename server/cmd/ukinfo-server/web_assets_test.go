package main

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWebAssetsRevisionAndCachePolicy(t *testing.T) {
	makeHandler := func(script string) (string, string) {
		t.Helper()
		handler, err := webHandler(fstest.MapFS{
			"index.html": {Data: []byte(`<script src="/app.js"></script><link href="/app.css" rel="stylesheet">`)},
			"app.js":     {Data: []byte(script)}, "app.css": {Data: []byte("body{}")},
		})
		if err != nil {
			t.Fatal(err)
		}
		index := httptest.NewRecorder()
		handler.ServeHTTP(index, httptest.NewRequest("GET", "/", nil))
		if index.Header().Get("Cache-Control") != "no-store" {
			t.Fatal(index.Header())
		}
		paths := regexp.MustCompile(`/app\.(?:js|css)\?v=[a-f0-9]{16}`).FindAllString(index.Body.String(), -1)
		if len(paths) != 2 {
			t.Fatal(index.Body.String())
		}
		for _, path := range paths {
			asset := httptest.NewRecorder()
			handler.ServeHTTP(asset, httptest.NewRequest("GET", path, nil))
			if asset.Code != 200 || !strings.Contains(asset.Header().Get("Cache-Control"), "immutable") {
				t.Fatal(asset.Code, asset.Header())
			}
		}
		old := httptest.NewRecorder()
		handler.ServeHTTP(old, httptest.NewRequest("GET", "/app.js", nil))
		if old.Header().Get("Cache-Control") != "no-cache" {
			t.Fatal(old.Header())
		}
		return paths[0], paths[1]
	}
	before, cssBefore := makeHandler("old build")
	after, cssAfter := makeHandler("new build")
	if before == after || cssBefore != cssAfter {
		t.Fatal("asset revisions do not track their contents")
	}
}
