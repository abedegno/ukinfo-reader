// Local closed-loop and scheduled-arrival HTTP load checks. No source records are logged.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type measures struct {
	Requests int     `json:"requests"`
	Errors   int     `json:"errors"`
	Median   float64 `json:"median_ms"`
	P95      float64 `json:"p95_ms"`
	P99      float64 `json:"p99_ms"`
	Max      float64 `json:"max_ms"`
	Wall     float64 `json:"wall_seconds"`
	QPS      float64 `json:"requests_per_second"`
}

func summarize(latencies []float64, errors int, wall time.Duration) measures {
	sort.Float64s(latencies)
	n := len(latencies)
	if n == 0 {
		return measures{Errors: errors, Wall: wall.Seconds()}
	}
	return measures{Requests: n, Errors: errors, Median: latencies[n/2], P95: latencies[int(math.Ceil(.95*float64(n)))-1], P99: latencies[int(math.Ceil(.99*float64(n)))-1], Max: latencies[n-1], Wall: wall.Seconds(), QPS: float64(n) / wall.Seconds()}
}
func main() {
	host := flag.String("url", "http://127.0.0.1:8080", "base API URL")
	duration := flag.Duration("duration", 10*time.Second, "duration of each closed-loop stage")
	rate := flag.Int("rate", 100, "scheduled requests per second")
	arrivals := flag.Duration("arrival-duration", 30*time.Second, "scheduled-arrival stage duration")
	hashPath := flag.String("cases", "", "private JSON array of {query,sha256} cases, validated before timing")
	out := flag.String("output", "", "data-free aggregate JSON report")
	flag.Parse()
	raw, err := os.ReadFile(*hashPath)
	if err != nil {
		panic(err)
	}
	type queryCase struct {
		Query  string `json:"query"`
		SHA256 string `json:"sha256"`
	}
	var cases []queryCase
	if err = json.Unmarshal(raw, &cases); err != nil {
		panic(err)
	}
	if len(cases) == 0 || *rate < 1 || *duration <= 0 || *arrivals <= 0 || *out == "" {
		panic("cases, output and positive duration/rate are required")
	}
	transport := &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 64, MaxConnsPerHost: 64, DisableCompression: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	url := *host + "/api/search?"
	request := func(index int) bool {
		response, err := client.Get(url + cases[index].Query)
		if err != nil {
			return false
		}
		body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
		response.Body.Close()
		if err != nil || response.StatusCode != 200 {
			return false
		}
		digest := sha256.Sum256(body)
		return hex.EncodeToString(digest[:]) == cases[index].SHA256
	}
	for i := range cases {
		if !request(i) {
			panic("warmup response failed validation")
		}
	}
	result := map[string]any{"cases": len(cases), "closed_loop": map[string]measures{}, "notes": []string{"Every response body is hash-checked against previously validated private cases.", "HTTP transfer and body hashing are timed; client JSON parsing is excluded.", "Scheduled-arrival latency includes scheduling and queue delay.", "This report alone does not establish production capacity; document corpus, hardware, quotas and warm/cold conditions separately."}}

	var sequence atomic.Uint64
	for _, concurrency := range []int{1, 8, 32} {
		type batch struct {
			times  []float64
			errors int
		}
		batches := make(chan batch, concurrency)
		startGate := make(chan struct{})
		var ready sync.WaitGroup
		ready.Add(concurrency)
		var deadline time.Time
		for worker := 0; worker < concurrency; worker++ {
			go func() {
				ready.Done()
				<-startGate
				values := make([]float64, 0, 4096)
				errors := 0
				for time.Now().Before(deadline) {
					index := int(sequence.Add(1)-1) % len(cases)
					start := time.Now()
					ok := request(index)
					values = append(values, float64(time.Since(start).Nanoseconds())/1e6)
					if !ok {
						errors++
					}
				}
				batches <- batch{values, errors}
			}()
		}
		ready.Wait()
		start := time.Now()
		deadline = start.Add(*duration)
		close(startGate)
		times := []float64{}
		errors := 0
		for worker := 0; worker < concurrency; worker++ {
			batch := <-batches
			times = append(times, batch.times...)
			errors += batch.errors
		}
		stats := summarize(times, errors, time.Since(start))
		result["closed_loop"].(map[string]measures)[fmt.Sprint(concurrency)] = stats
		fmt.Printf("%s clients=%d requests=%d p95=%.3fms errors=%d\n", *host, concurrency, stats.Requests, stats.P95, stats.Errors)
	}
	type scheduled struct {
		index int
		at    time.Time
	}
	jobs := make(chan scheduled, 32)
	scheduledCount := int(arrivals.Seconds() * float64(*rate))
	times := make(chan float64, scheduledCount)
	var errors atomic.Int64
	var workers sync.WaitGroup
	for worker := 0; worker < 16; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				if !request(job.index) {
					errors.Add(1)
				}
				times <- float64(time.Since(job.at).Nanoseconds()) / 1e6
			}
		}()
	}
	start := time.Now()
	dropped := 0
	for i := 0; i < scheduledCount; i++ {
		at := start.Add(time.Duration(float64(i) * float64(time.Second) / float64(*rate)))
		if delay := time.Until(at); delay > 0 {
			time.Sleep(delay)
		}
		select {
		case jobs <- scheduled{i % len(cases), at}:
		default:
			dropped++
		}
	}
	close(jobs)
	workers.Wait()
	close(times)
	latencies := []float64{}
	for value := range times {
		latencies = append(latencies, value)
	}
	result["scheduled_arrivals"] = map[string]any{"rate_per_second": *rate, "scheduled_requests": scheduledCount, "load_generator_drops": dropped, "stats": summarize(latencies, int(errors.Load()), time.Since(start))}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile(*out, append(data, '\n'), 0600); err != nil {
		panic(err)
	}
}
