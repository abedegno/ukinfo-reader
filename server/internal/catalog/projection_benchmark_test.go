package catalog

import (
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"strings"
	"testing"
)

func testProjectionBenchmark(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `CREATE SCHEMA ukinfo_projection_spike;
 CREATE TABLE ukinfo_projection_spike.build AS SELECT id dataset_id,entries expected,now() finished_at FROM datasets WHERE key='large-address';
 CREATE TABLE ukinfo_projection_spike.entries AS
 SELECT e.*,r.group_key,r.sic,r.industry,b.location_ids[(e.id-1)%128+1] location_id FROM entries e JOIN research_keys r USING(dataset_id,id)
 JOIN address_record_blocks b ON b.dataset_id=e.dataset_id AND b.block_id=(e.id-1)/128
 WHERE e.dataset_id=(SELECT dataset_id FROM ukinfo_projection_spike.build);`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DROP SCHEMA ukinfo_projection_spike CASCADE")
	cases := []ProjectionCase{{"surname=Widebudget&town=Large+Town"}, {"postcode=CC1&county=Old*"}, {"surname=Missingmatch&town=Large+Town"}, {"kind=business&industry=missing*"}}
	report, err := BenchmarkProjection(ctx, db, cases, "large-address", 2, 3)
	if err != nil || !report.Complete || report.Failures != 0 || report.Mismatches != 0 || len(report.Observations) != 16 {
		t.Fatal("projection comparison", len(report.Observations), report.Failures, report.Mismatches, err)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"Widebudget", "Large", "CC1", "Old", "query", "digest", "source_id", "group_key"} {
		if strings.Contains(string(raw), private) {
			t.Fatal("private data in report", private)
		}
	}
	if _, err = db.Exec(ctx, "UPDATE ukinfo_projection_spike.entries SET name_id=0 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	report, err = BenchmarkProjection(ctx, db, cases[:1], "large-address", 1, 1)
	if err != nil || report.Mismatches != 1 {
		t.Fatal("corrupt projection was accepted", err)
	}
	if _, err = db.Exec(ctx, `UPDATE ukinfo_projection_spike.entries p SET name_id=e.name_id FROM entries e WHERE e.dataset_id=p.dataset_id AND e.id=p.id AND p.id=1;
 UPDATE ukinfo_projection_spike.entries SET group_key=decode(repeat('ff',32),'hex') WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	report, err = BenchmarkProjection(ctx, db, cases[:1], "large-address", 1, 1)
	if err != nil || report.Mismatches != 1 {
		t.Fatal("corrupt grouping identity was accepted", err)
	}
}

func testPositiveProjectionBenchmark(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `CREATE SCHEMA ukinfo_projection_spike;
 CREATE TABLE ukinfo_projection_spike.build AS SELECT id dataset_id,entries expected,now() finished_at FROM datasets WHERE key='business-address-work';
 CREATE TABLE ukinfo_projection_spike.entries AS
 SELECT e.*,r.group_key,r.sic,r.industry,b.location_ids[(e.id-1)%128+1] location_id FROM entries e JOIN research_keys r USING(dataset_id,id)
 JOIN address_record_blocks b ON b.dataset_id=e.dataset_id AND b.block_id=(e.id-1)/128
 WHERE e.dataset_id=(SELECT dataset_id FROM ukinfo_projection_spike.build);`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec(ctx, "DROP SCHEMA ukinfo_projection_spike CASCADE")
	cases := []ProjectionCase{{"kind=business&sic=9999"}, {"kind=business&sic=12*&town=Other+Business+Town"}, {"kind=business&industry=Other+widgets"}, {"kind=business&sic=missing"}}
	r, err := BenchmarkProjection(ctx, db, cases, "business-address-work", 2, 3)
	if err != nil || !r.Complete || r.Failures != 0 || r.Mismatches != 0 || len(r.Observations) != 16 {
		t.Fatal("positive classification comparison", len(r.Observations), r.Failures, r.Mismatches, err)
	}
	for _, o := range r.Observations {
		if (o.Case == 3 && o.Rows != 0) || (o.Case != 3 && o.Rows != 50) {
			t.Fatal("classification result cardinality changed", o.Case, o.Rows)
		}
	}
}
