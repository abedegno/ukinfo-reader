package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Database-wide counters are useful only for an isolated private source. Flushing
// our idle reader backends does not reset statistics, write data, or evict caches.
type ioStats struct {
	Available bool    `json:"available"`
	Reads     int64   `json:"read_blocks"`
	Hits      int64   `json:"hit_blocks"`
	ReadMS    float64 `json:"read_ms"`
	TempBytes int64   `json:"temp_bytes"`
}

func (s ioStats) minus(b ioStats) ioStats {
	if !s.Available || !b.Available || s.Reads < b.Reads || s.Hits < b.Hits || s.ReadMS < b.ReadMS || s.TempBytes < b.TempBytes {
		return ioStats{}
	}
	return ioStats{true, s.Reads - b.Reads, s.Hits - b.Hits, s.ReadMS - b.ReadMS, s.TempBytes - b.TempBytes}
}
func readIO(ctx context.Context, db *pgxpool.Pool) ioStats {
	ready := true
	for _, c := range db.AcquireAllIdle(ctx) {
		if _, err := c.Exec(ctx, "SELECT pg_stat_force_next_flush()"); err != nil {
			ready = false
		}
		c.Release()
	}
	var out ioStats
	err := db.QueryRow(ctx, `SELECT blks_read,blks_hit,blk_read_time,temp_bytes FROM pg_stat_database WHERE datname=current_database()`).Scan(&out.Reads, &out.Hits, &out.ReadMS, &out.TempBytes)
	out.Available = ready && err == nil
	return out
}
