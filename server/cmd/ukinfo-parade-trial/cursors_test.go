package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
)

type privateTransport func(*http.Request) (*http.Response, error)

func (f privateTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPrivateCursorRequest(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: privateTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "ukinfo-parade-preview-candidate:8080" || r.URL.Query().Get("cursor") != "opaque+/=" || r.URL.Query().Get("surname") != "O'Neill" {
			t.Fatal("incorrect private continuation request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"items":[{"id":1}],"next_cursor":"next"}`)), Header: make(http.Header)}, nil
	})}
	c := catalog.ParadeTrialCase{Path: "/api/search", Query: "dataset=all&surname=O%27Neill", DigestMode: "cursor-neutral-v1"}
	p := privateRead(context.Background(), client, 1, c, "opaque+/=")
	if p.status != 200 || p.hash == "" || p.next != "next" {
		t.Fatal("lost response content or continuation")
	}
	c.Path = "@external.example/path"
	if p = privateRead(context.Background(), client, 1, c, ""); p.status != 0 || calls != 1 {
		t.Fatal("unsafe path reached transport")
	}
	c.Path = "/api/search"
	if p = privateRead(context.Background(), client, 5, c, ""); p.status != 0 || calls != 1 {
		t.Fatal("invalid backend reached transport")
	}
}
