package catalog

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
)

func testCatalogueManagement(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, key := range []string{"managed-remove", "managed-keep"} {
		if _, err := ImportWithOptions(ctx, db, strings.NewReader(fixture(key, true)), ImportOptions{IndexAddresses: true}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := CatalogueInventory(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	var target ManagedDataset
	for _, d := range before.Datasets {
		if d.Key == "managed-remove" {
			target = d
		}
	}
	if target.ID < 1 || target.Bytes <= 0 || !target.AddressReady || before.SharedBytes <= 0 {
		t.Fatalf("incomplete inventory: %+v", target)
	}
	if err = RemoveDataset(ctx, db, target.Key, target.ID+1); err == nil {
		t.Fatal("stale confirmation accepted")
	}
	var exists bool
	if err = db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM datasets WHERE key=$1)", target.Key).Scan(&exists); err != nil || !exists {
		t.Fatal("stale removal changed dataset", err)
	}
	// An ongoing index build excludes deletion; neither operation should win by
	// destroying tables under the other.
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(742918306,hashtext(current_schema()))"); err != nil {
		t.Fatal(err)
	}
	if err = RemoveDataset(ctx, db, target.Key, target.ID); err == nil {
		t.Fatal("removal ignored index lock")
	}
	_ = tx.Rollback(ctx)
	if err = RemoveDataset(ctx, db, target.Key, target.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM address_members WHERE dataset_id=$1", target.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("address membership retained", err)
	}
	for _, table := range []string{"address_record_blocks", "address_record_indexes"} {
		if err = db.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE dataset_id=$1", target.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s retained removed dataset: count=%d err=%v", table, count, err)
		}
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM datasets WHERE key='managed-keep'").Scan(&count); err != nil || count != 1 {
		t.Fatal("other edition changed", err)
	}
	if _, err = CatalogueInventory(ctx, db); err != nil {
		t.Fatal("shared tables damaged", err)
	}
	if _, err = Import(ctx, db, strings.NewReader(fixture(target.Key, true))); err != nil {
		t.Fatal("reimport failed", err)
	}
	var newID int16
	if err = db.QueryRow(ctx, "SELECT id FROM datasets WHERE key=$1", target.Key).Scan(&newID); err != nil || newID == target.ID {
		t.Fatal("removed dataset identity was reused", err)
	}
	if err = RemoveDataset(ctx, db, target.Key, target.ID); err == nil {
		t.Fatal("old confirmation removed reimport")
	}
}
