package catalog

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

var errRecordIntegrity = errors.New("source record integrity failure")

type responseError struct{ code, message string }

func (e *responseError) Error() string { return e.message }
func isQueryTimeout(err error) bool {
	var pgErr *pgconn.PgError
	return errors.Is(err, context.DeadlineExceeded) || pgconn.Timeout(err) || (errors.As(err, &pgErr) && pgErr.Code == "57014")
}
func searchReadError(err error) error {
	if isQueryTimeout(err) {
		return &responseError{"query_timeout", "Search timed out; narrow the search"}
	}
	return &responseError{"unavailable", "Search temporarily unavailable; retry shortly"}
}
func sourceReadError(err error) error {
	if isQueryTimeout(err) {
		return &responseError{"query_timeout", "Source reading timed out. Retry or narrow the search."}
	}
	if errors.Is(err, errRecordIntegrity) {
		return &responseError{"catalogue_integrity", "Source records failed integrity checks; the catalogue needs verification."}
	}
	return &responseError{"unavailable", "Source records are temporarily unavailable. Please retry shortly."}
}
func failError(w http.ResponseWriter, status int, err error) {
	var typed *responseError
	if errors.As(err, &typed) {
		writeFailure(w, status, typed.code, typed.message)
		return
	}
	fail(w, status, err.Error())
}
func writeFailure(w http.ResponseWriter, status int, code, message string) {
	if observer, ok := w.(interface{ setFailureCode(string) }); ok {
		observer.setFailureCode(code)
	}
	payload := map[string]string{"error": message, "code": code}
	if id := w.Header().Get("X-Request-ID"); id != "" {
		payload["request_id"] = id
	}
	respond(w, status, payload)
}
func newRequestID() string {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(raw[:])
}
func recordIntegrity(message string) error { return fmt.Errorf("%w: %s", errRecordIntegrity, message) }
