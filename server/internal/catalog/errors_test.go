package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestSourceFailureCategories(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{{context.DeadlineExceeded, "query_timeout"}, {&pgconn.PgError{Code: "57014"}, "query_timeout"}, {recordIntegrity("private detail"), "catalogue_integrity"}, {errors.New("private connection detail"), "unavailable"}} {
		w := httptest.NewRecorder()
		w.Header().Set("X-Request-ID", newRequestID())
		failError(w, 503, sourceReadError(tc.err))
		var body map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["code"] != tc.code || len(body["request_id"]) != 24 || strings.Contains(w.Body.String(), "private") {
			t.Fatal(w.Body.String())
		}
	}
}
