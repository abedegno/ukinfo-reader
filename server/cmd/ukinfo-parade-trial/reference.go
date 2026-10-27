package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

type referenceRow struct {
	Case   int    `json:"case"`
	Page   int    `json:"page"`
	Cursor string `json:"next_cursor"`
	Digest string `json:"sha256"`
}
type nativeReferenceFile struct {
	Cases int            `json:"cases"`
	Rows  []referenceRow `json:"rows"`
}

func referenceCases(path string) ([]catalog.ParadeTrialCase, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, errors.New("private cases unavailable")
	}
	var cases []catalog.ParadeTrialCase
	if json.Unmarshal(raw, &cases) != nil || len(cases) < 1 || len(cases) > 500 {
		return nil, errors.New("invalid private cases")
	}
	return cases, nil
}

// This stage is built with an explicit private Go overlay raising only the
// reference handler/SQL budgets. It does not listen on a socket or alter serving
// configuration. Full responses stay in NAS memory; only hashes/cursors are saved.
func nativeReference(ctx context.Context, path, ordinals, output, report string, full bool) error {
	if report == "" || report == output {
		return errors.New("separate aggregate report required")
	}
	cases, e := referenceCases(path)
	if e != nil {
		return e
	}
	cfg, e := pgxpool.ParseConfig(os.Getenv("SOURCE_DATABASE_URL"))
	if e != nil || os.Getenv("SOURCE_DATABASE_URL") == "" {
		return errors.New("explicit private reader required")
	}
	cfg.MaxConns = 2
	cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "120000"
	cfg.ConnConfig.RuntimeParams["application_name"] = "ukinfo-native-reference"
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return errors.New("reference reader unavailable")
	}
	defer db.Close()
	refs := nativeReferenceFile{Cases: len(cases), Rows: []referenceRow{}}
	observations := []map[string]any{}
	failures := 0
	parts := strings.Split(ordinals, ",")
	if len(parts) < 1 || len(parts) > 4 {
		return errors.New("choose 1..4 reference cases")
	}
	for _, part := range parts {
		ci, e := strconv.Atoi(part)
		if e != nil || ci < 0 || ci >= len(cases) || cases[ci].Path != "/api/search" {
			return errors.New("invalid reference ordinal")
		}
		v, e := url.ParseQuery(cases[ci].Query)
		if e != nil {
			return errors.New("invalid private reference")
		}
		start := time.Now()
		var pages [][]byte
		if full {
			pages, e = fullNativePages(ctx, db, v)
		} else {
			var boundDataset string
			var boundID int32
			boundDataset, boundID, e = referenceBoundary(ctx, cases[ci])
			if e == nil {
				pages, e = catalog.NativeOrderedPrefixReference(ctx, db, v, boundDataset, boundID)
			}
		}
		elapsed := float64(time.Since(start).Microseconds()) / 1000
		status := 200
		if e != nil || len(pages) < 1 || len(pages) > 2 || (!full && len(pages) != 2) {
			status = 503
			failures++
		}
		observation := map[string]any{"case": ci, "pages": len(pages), "status": status, "milliseconds": elapsed}
		if e != nil {
			observation["reason"] = "reference_failed"
			switch e.Error() {
			case "reference vocabulary prefix exceeded safety bound", "reference address population did not complete", "reference address population unavailable", "reference source batch unavailable", "reference source batch unreadable", "reference vocabulary unavailable", "reference vocabulary incomplete", "reference prefix did not complete", "native prefix ordering failed", "complete reverse-address reference required":
				observation["reason"] = e.Error()
			}
		}
		observations = append(observations, observation)
		if status != 200 {
			continue
		}
		cursor := ""
		for i, body := range pages {
			hash, e := digest(body, cases[ci].DigestMode)
			if e != nil {
				return errors.New("invalid reference response")
			}
			refs.Rows = append(refs.Rows, referenceRow{ci, i + 1, cursor, hash})
			var fields struct {
				Next string `json:"next_cursor"`
			}
			if json.Unmarshal(body, &fields) != nil {
				return errors.New("invalid reference response")
			}
			cursor = fields.Next
		}

	}
	if e := save(output, refs); e != nil {
		return e
	}
	if e := save(report, map[string]any{"cases": len(parts), "pages": len(refs.Rows), "errors": failures, "complete": failures == 0, "performance_measurement": false, "observations": observations}); e != nil {
		return e
	}
	if failures > 0 {
		return errors.New("native reference did not finish")
	}
	return nil
}

// Full execution under the offline overlay, independent of candidate ordering.
// The normal serving executable has no expanded deadline or reference route.
func fullNativePages(ctx context.Context, db *pgxpool.Pool, values url.Values) ([][]byte, error) {
	handler := catalog.NewAPI(db).Handler()
	pages := [][]byte{}
	for page := 0; page < 2; page++ {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/search?"+values.Encode(), nil).WithContext(ctx))
		if w.Code != 200 {
			return nil, errors.New("full native reference did not complete")
		}
		body := append([]byte(nil), w.Body.Bytes()...)
		var fields struct {
			Next string `json:"next_cursor"`
		}
		if json.Unmarshal(body, &fields) != nil {
			return nil, errors.New("invalid native reference response")
		}
		pages = append(pages, body)
		if fields.Next == "" {
			break
		}
		values.Set("cursor", fields.Next)
	}
	return pages, nil
}

func compareSnapshotReference(reference, snapshot, output string) error {
	s, e := readSnapshot(snapshot)
	if e != nil {
		return e
	}
	raw, e := os.ReadFile(reference)
	if e != nil {
		return errors.New("private source reference unavailable")
	}
	var refs nativeReferenceFile
	if json.Unmarshal(raw, &refs) != nil || refs.Cases != s.Cases || len(refs.Rows) < 1 || len(refs.Rows) > 8 {
		return errors.New("invalid bounded source reference")
	}
	first := map[int]string{}
	for _, r := range refs.Rows {
		if r.Page == 1 && r.Case >= 0 && r.Case < s.Cases && len(r.Digest) == 64 {
			if _, duplicate := first[r.Case]; duplicate {
				return errors.New("duplicate source reference")
			}
			first[r.Case] = r.Digest
		}
	}
	failures, mismatches := 0, 0
	for _, r := range s.Rows {
		want, ok := first[r.Case]
		if !ok {
			return errors.New("snapshot has no source reference")
		}
		if !r.Success {
			failures++
		} else if r.Digest != want {
			mismatches++
		}
	}
	if e = save(output, map[string]any{"complete": true, "requests": len(s.Rows), "errors": failures, "mismatches": mismatches}); e != nil {
		return e
	}
	if failures+mismatches > 0 {
		return errors.New("source snapshot comparison failed")
	}
	return nil
}
func compareReference(ctx context.Context, path, reference, output string, hosted bool) error {
	cases, e := referenceCases(path)
	if e != nil {
		return e
	}
	raw, e := os.ReadFile(reference)
	if e != nil {
		return errors.New("private reference unavailable")
	}
	var refs nativeReferenceFile
	if json.Unmarshal(raw, &refs) != nil || refs.Cases != len(cases) || len(refs.Rows) < 1 || len(refs.Rows) > 8 {
		return errors.New("invalid native reference")
	}
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 2}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	own := map[int]string{}
	observations := []map[string]any{}
	mismatches, failures := 0, 0
	for _, r := range refs.Rows {
		if r.Case < 0 || r.Case >= len(cases) || r.Page < 1 || r.Page > 2 || len(r.Digest) != 64 {
			return errors.New("invalid bounded native reference")
		}
		for _, mode := range []string{"candidate_cursor", "native_cursor"} {
			if hosted && len(observations) > 0 {
				select {
				case <-ctx.Done():
					return errors.New("acceptance cancelled")
				case <-time.After(600 * time.Millisecond):
				}
			}
			cursor := r.Cursor
			if mode == "candidate_cursor" {
				cursor = own[r.Case]
			}
			p := privateReadTarget(ctx, client, 1, cases[r.Case], cursor, hosted)
			match := p.hash != "" && p.hash == r.Digest
			if p.hash == "" {
				failures++
			} else if !match {
				mismatches++
			}
			if mode == "candidate_cursor" {
				own[r.Case] = p.next
			}
			observations = append(observations, map[string]any{"case": r.Case, "page": r.Page, "mode": mode, "match": match, "status": p.status, "milliseconds": p.ms})
		}
	}
	if e := save(output, map[string]any{"complete": true, "requests": len(observations), "errors": failures, "mismatches": mismatches, "observations": observations}); e != nil {
		return e
	}
	if failures+mismatches > 0 {
		return errors.New("native reference comparison failed")
	}
	return nil
}

// Only a source identity on page three is used as the oracle's inclusive ordering
// bound. The oracle enumerates source prefixes independently of candidate IDs.
func referenceBoundary(ctx context.Context, c catalog.ParadeTrialCase) (string, int32, error) {
	client := &http.Client{Timeout: 8 * time.Second, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 1}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	v, e := url.ParseQuery(c.Query)
	if e != nil {
		return "", 0, errors.New("invalid boundary query")
	}
	for page := 1; page <= 3; page++ {
		req, e := http.NewRequestWithContext(ctx, "GET", privateBases[1]+"/api/search?"+v.Encode(), nil)
		if e != nil {
			return "", 0, errors.New("invalid boundary request")
		}
		r, e := client.Do(req)
		if e != nil {
			return "", 0, errors.New("boundary request failed")
		}
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, (8<<20)+1))
		r.Body.Close()
		if readErr != nil || r.StatusCode != 200 || len(raw) > 8<<20 {
			return "", 0, errors.New("boundary request failed")
		}
		var fields struct {
			Next  string `json:"next_cursor"`
			Items []struct {
				Dataset string `json:"dataset"`
				ID      int64  `json:"id"`
			} `json:"items"`
		}
		if json.Unmarshal(raw, &fields) != nil || len(fields.Items) == 0 {
			return "", 0, errors.New("boundary requires lookahead")
		}
		if page == 3 {
			p := fields.Items[0]
			if p.Dataset == "" || p.ID < 1 || p.ID > 2147483647 {
				return "", 0, errors.New("invalid source boundary")
			}
			return p.Dataset, int32(p.ID), nil
		}
		if fields.Next == "" {
			return "", 0, errors.New("boundary requires three pages")
		}
		v.Set("cursor", fields.Next)
	}
	return "", 0, errors.New("boundary unavailable")
}
