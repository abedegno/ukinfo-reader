package main

import (
	"context"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"sort"
	"strings"
	"sync"
	"time"
)

func profilePhase(name string) string {
	if !strings.HasPrefix(name, "search.") {
		return "metadata"
	}
	switch phase := strings.TrimPrefix(name, "search."); phase {
	case "parade_inventory", "parade_candidates", "parade_pair", "geography", "vocabulary", "source_probe", "address_probe", "address_blocks", "address_dictionary", "candidates", "ordered", "group_members", "records":
		return phase
	}
	return "metadata"
}

// Sum the union, not the sum, of overlapping SQL intervals. Rows remain private
// in memory; only timings and fixed stage names reach the report.
func queryWallMS(rows []*query, phase string) float64 {
	type interval struct{ start, end time.Time }
	intervals := []interval{}
	for _, q := range rows {
		if q.Phase == phase {
			intervals = append(intervals, interval{q.Started, q.Started.Add(time.Duration(q.Milliseconds * float64(time.Millisecond)))})
		}
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].start.Before(intervals[j].start) })
	var total time.Duration
	var start, end time.Time
	for _, v := range intervals {
		if end.IsZero() {
			start, end = v.start, v.end
			continue
		}
		if v.start.After(end) {
			total += end.Sub(start)
			start, end = v.start, v.end
		} else if v.end.After(end) {
			end = v.end
		}
	}
	if !end.IsZero() {
		total += end.Sub(start)
	}
	return float64(total) / float64(time.Millisecond)
}

type phaseRecorder struct {
	mu sync.Mutex
	ms map[string]float64
}

func (*phaseRecorder) OnStart(context.Context, sdktrace.ReadWriteSpan) {}
func (p *phaseRecorder) OnEnd(s sdktrace.ReadOnlySpan) {
	phase := profilePhase(s.Name())
	if phase == "metadata" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.ms == nil {
		p.ms = map[string]float64{}
	}
	p.ms[phase] += float64(s.EndTime().Sub(s.StartTime())) / float64(time.Millisecond)
}
func (*phaseRecorder) Shutdown(context.Context) error   { return nil }
func (*phaseRecorder) ForceFlush(context.Context) error { return nil }
func (p *phaseRecorder) reset()                         { p.mu.Lock(); defer p.mu.Unlock(); p.ms = map[string]float64{} }
func (p *phaseRecorder) snapshot() map[string]float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := map[string]float64{}
	for k, v := range p.ms {
		out[k] = v
	}
	return out
}

type cpuUsage struct {
	Available bool    `json:"available"`
	UserMS    float64 `json:"user_ms"`
	SystemMS  float64 `json:"system_ms"`
}

func (c cpuUsage) minus(before cpuUsage) cpuUsage {
	return cpuUsage{c.Available && before.Available, c.UserMS - before.UserMS, c.SystemMS - before.SystemMS}
}
