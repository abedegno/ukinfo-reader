package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestLifecycle(t *testing.T) {
	bin := os.Getenv("UKINFO_TEST_PG_BIN")
	if bin == "" {
		t.Skip("set UKINFO_TEST_PG_BIN to native PostgreSQL 18 binaries")
	}
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "original")
	first, err := Start(ctx, dir, bin)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	db, err := pgx.Connect(ctx, first.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, "CREATE TABLE retained (value text); INSERT INTO retained VALUES ('survives restart')"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close(ctx)
	if other, e := Start(ctx, dir, bin); e == nil {
		other.Close()
		t.Fatal("second owner accepted")
	}
	first.Close()
	first.Close() // shutdown is idempotent
	relocated := filepath.Join(filepath.Dir(dir), "relocated")
	if err = os.Rename(dir, relocated); err != nil {
		t.Fatal(err)
	}
	second, err := Start(ctx, relocated, bin)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	db, err = pgx.Connect(ctx, second.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)
	var value string
	if err = db.QueryRow(ctx, "SELECT value FROM retained").Scan(&value); err != nil || value != "survives restart" {
		t.Fatal("persistence", value, err)
	}
}
