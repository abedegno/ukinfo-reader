package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"time"
)

// checkHosted reaches only the Docker service, with no proxy or redirects. Saved
// real queries and digests never travel through the public hostname or Internet.
// Twelve evenly spread, previously matched references keep acceptance bounded
// below the public per-client quota; this is not another load benchmark.
func checkHosted(ctx context.Context, path, output string) error {
	raw, e := os.ReadFile(path)
	if e != nil {
		return errors.New("private references unavailable")
	}
	var cases []loadCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) < 12 || len(cases) > 500 {
		return errors.New("invalid bounded references")
	}
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 1}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	observations := []snapshotObservation{}
	matched := 0
	for i := 0; i < 12; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return errors.New("acceptance cancelled")
			case <-time.After(600 * time.Millisecond):
			}
		}
		n := i * (len(cases) - 1) / 11
		c := cases[n]
		if c.Path != "/api/search" {
			return errors.New("acceptance permits search references only")
		}
		started := time.Now()
		req, e := http.NewRequestWithContext(ctx, "GET", "http://ukinfo:8080"+c.Path+"?"+c.Query, nil)
		if e != nil {
			return errors.New("invalid private reference")
		}
		req.Host = "ukinfo.6153112.xyz"
		o := snapshotObservation{Case: n}
		r, e := client.Do(req)
		if e == nil {
			o.Status = r.StatusCode
			body, readErr := io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
			r.Body.Close()
			if r.StatusCode == 200 && readErr == nil && len(body) <= 8<<20 {
				hash, e := digest(body, c.DigestMode)
				if e == nil && hash == c.SHA256 {
					o.Success = true
					matched++
				}
			}
		}
		o.MS = float64(time.Since(started).Microseconds()) / 1000
		observations = append(observations, o)
	}
	if e := save(output, map[string]any{"complete": true, "requests": 12, "matched": matched, "mismatches_or_errors": 12 - matched, "internal_network_only": true, "observations": observations}); e != nil {
		return e
	}
	if matched != 12 {
		return errors.New("hosted reference acceptance failed")
	}
	return nil
}
