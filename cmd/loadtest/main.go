package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type created struct {
	ID string `json:"id"`
}

func main() {
	base := flag.String("url", "http://localhost:18080", "API base URL")
	jobs := flag.Int("jobs", 500, "jobs to submit")
	concurrency := flag.Int("concurrency", 25, "parallel clients")
	wait := flag.Bool("wait", true, "wait for terminal states")
	timeout := flag.Duration("timeout", 2*time.Minute, "whole run timeout")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	client := &http.Client{Timeout: 15 * time.Second}
	key := os.Getenv("FORGEQUEUE_API_KEY")
	ids := make(chan string, *jobs)
	work := make(chan int)
	lat := make([]float64, 0, *jobs)
	var mu sync.Mutex
	var accepted, failed atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	for n := 0; n < *concurrency; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				body := []byte(fmt.Sprintf(`{"type":"echo","payload":{"sequence":%d},"idempotency_key":"load-%d-%d"}`, i, start.UnixNano(), i))
				t := time.Now()
				req, _ := http.NewRequestWithContext(ctx, http.MethodPost, *base+"/v1/jobs", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				if key != "" {
					req.Header.Set("Authorization", "Bearer "+key)
				}
				resp, e := client.Do(req)
				elapsed := float64(time.Since(t).Microseconds()) / 1000
				if e != nil {
					failed.Add(1)
					continue
				}
				b, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if resp.StatusCode/100 != 2 {
					failed.Add(1)
					continue
				}
				var c created
				if json.Unmarshal(b, &c) != nil {
					failed.Add(1)
					continue
				}
				mu.Lock()
				lat = append(lat, elapsed)
				mu.Unlock()
				accepted.Add(1)
				ids <- c.ID
			}
		}()
	}
	go func() {
		for i := 0; i < *jobs; i++ {
			select {
			case work <- i:
			case <-ctx.Done():
				break
			}
		}
		close(work)
		wg.Wait()
		close(ids)
	}()
	all := []string{}
	for id := range ids {
		all = append(all, id)
	}
	var completed int64
	if *wait {
		pending := map[string]bool{}
		for _, id := range all {
			pending[id] = true
		}
		for len(pending) > 0 && ctx.Err() == nil {
			for id := range pending {
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, *base+"/v1/jobs/"+id, nil)
				if key != "" {
					req.Header.Set("Authorization", "Bearer "+key)
				}
				resp, e := client.Do(req)
				if e != nil {
					continue
				}
				var state struct {
					Status string `json:"status"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&state)
				resp.Body.Close()
				switch state.Status {
				case "succeeded", "failed", "dead", "cancelled":
					delete(pending, id)
					completed++
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	sort.Float64s(lat)
	duration := time.Since(start).Seconds()
	r := map[string]any{"jobs": *jobs, "concurrency": *concurrency, "accepted": accepted.Load(), "completed": completed, "failed_submissions": failed.Load(), "duration_seconds": duration, "throughput_jobs_per_second": float64(accepted.Load()) / duration, "submit_latency_ms": map[string]float64{"p50": pct(lat, .50), "p95": pct(lat, .95), "p99": pct(lat, .99)}}
	_ = json.NewEncoder(os.Stdout).Encode(r)
	if failed.Load() > 0 || ctx.Err() != nil {
		os.Exit(1)
	}
}
func pct(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	i := int(float64(len(v)-1) * p)
	return v[i]
}
