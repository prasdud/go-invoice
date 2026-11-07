// Command stress is a zero-dependency closed-loop load generator for
// go-invoice. It ramps fixed worker counts until the service sheds load and
// reports throughput, latency percentiles, and server resource signals per
// step.
//
//	go run ./cmd/stress -url http://localhost:8080 -key changeme -engine basic
//	go run ./cmd/stress -engine html -cjk -steps 1,2,4,8,16,32
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	baseURL := flag.String("url", "http://localhost:8080", "server base URL")
	apiKey := flag.String("key", envOr("API_KEY", "changeme"), "API key")
	engine := flag.String("engine", "basic", "engine: basic|html")
	stepsCSV := flag.String("steps", "1,2,4,8,16,32,64,128", "worker counts to ramp through")
	stepDur := flag.Duration("duration", 15*time.Second, "duration per step")
	stopRate := flag.Float64("stop-error-rate", 0.10, "stop ramping when 503+error rate exceeds this")
	cjk := flag.Bool("cjk", false, "use a CJK payload (for the html engine)")
	flag.Parse()

	steps, err := parseSteps(*stepsCSV)
	if err != nil {
		fatal(err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConnsPerHost: 512,
			DisableCompression:  true,
		},
	}
	if err := checkHealth(client, *baseURL); err != nil {
		fatal(fmt.Errorf("server not reachable: %w", err))
	}

	body := mustJSON(payload(*engine, *cjk))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Printf("stress: %s  engine=%s  step=%s\n\n", *baseURL, *engine, *stepDur)
	fmt.Printf("%5s %8s %7s %6s %9s %9s %7s %7s %7s %12s %9s\n",
		"conc", "ok", "503", "err", "req/s", "inv/min", "p50ms", "p90ms", "p99ms", "goroutines", "rss_mb")

	var best struct {
		conc   int
		perMin float64
	}
	for _, conc := range steps {
		res := runStep(ctx, client, *baseURL, *apiKey, body, conc, *stepDur)
		goroutines, rssMB := serverMetrics(client, *baseURL)
		fmt.Printf("%5d %8d %7d %6d %9.0f %9.0f %7.1f %7.1f %7.1f %12.0f %9.1f\n",
			conc, res.ok, res.over, res.errs, res.rps, res.perMin,
			toMS(res.p50), toMS(res.p90), toMS(res.p99), goroutines, rssMB)

		if res.ok > 0 && res.perMin > best.perMin {
			best.conc, best.perMin = conc, res.perMin
		}

		total := res.ok + res.over + res.errs
		if total > 0 && float64(res.over+res.errs)/float64(total) > *stopRate {
			fmt.Printf("\nknee reached at concurrency %d: %.0f%% shed or failed\n",
				conc, 100*float64(res.over+res.errs)/float64(total))
			break
		}
		if ctx.Err() != nil {
			break
		}
	}

	if best.perMin > 0 {
		fmt.Printf("\nbest sustained: %.0f invoices/min at concurrency %d\n", best.perMin, best.conc)
	}
}

type stepResult struct {
	ok, over, errs int64
	rps, perMin    float64
	p50, p90, p99  time.Duration
}

func runStep(ctx context.Context, client *http.Client, baseURL, key string, body []byte, workers int, dur time.Duration) stepResult {
	var ok, over, errs atomic.Int64
	lats := make([][]time.Duration, workers)
	var wg sync.WaitGroup

	stepCtx, cancel := context.WithTimeout(ctx, dur)
	defer cancel()

	start := time.Now()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			var local []time.Duration
			for stepCtx.Err() == nil {
				req, err := http.NewRequestWithContext(stepCtx, http.MethodPost, baseURL+"/invoices", bytes.NewReader(body))
				if err != nil {
					errs.Add(1)
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-API-Key", key)

				t0 := time.Now()
				resp, err := client.Do(req)
				elapsed := time.Since(t0)
				if err != nil {
					if stepCtx.Err() == nil {
						errs.Add(1)
					}
					continue
				}
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				switch resp.StatusCode {
				case http.StatusOK:
					ok.Add(1)
					local = append(local, elapsed)
				case http.StatusServiceUnavailable:
					over.Add(1)
				default:
					errs.Add(1)
				}
			}
			lats[w] = local
		}(w)
	}
	wg.Wait()
	wall := time.Since(start)

	var all []time.Duration
	for _, l := range lats {
		all = append(all, l...)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })

	res := stepResult{
		ok: ok.Load(), over: over.Load(), errs: errs.Load(),
		p50: pct(all, 50), p90: pct(all, 90), p99: pct(all, 99),
	}
	if wall > 0 {
		res.rps = float64(res.ok+res.over+res.errs) / wall.Seconds()
		res.perMin = float64(res.ok) / wall.Minutes()
	}
	return res
}

func pct(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if i < 0 {
		i = 0
	}
	return sorted[i]
}

func toMS(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// serverMetrics scrapes a few series from the service's /metrics endpoint.
func serverMetrics(client *http.Client, baseURL string) (goroutines, rssMB float64) {
	resp, err := client.Get(baseURL + "/metrics")
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(line, "go_goroutines "):
			goroutines = parseValue(line)
		case strings.HasPrefix(line, "process_resident_memory_bytes "):
			rssMB = parseValue(line) / (1 << 20)
		}
	}
	return goroutines, rssMB
}

func parseValue(line string) float64 {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return 0
	}
	v, _ := strconv.ParseFloat(fields[1], 64)
	return v
}

func payload(engine string, cjk bool) map[string]any {
	p := map[string]any{
		"engine": engine,
		"payload": map[string]any{
			"invoice_number": "INV-STRESS",
			"date":           "2026-11-07",
			"bill_to":        "Stress Test Co",
			"items": []map[string]any{
				{"description": "Widget A", "quantity": 3, "rate": 19.99},
				{"description": "Widget B", "quantity": 1, "rate": 149.0},
			},
			"tax_rate": 8.5,
		},
	}
	if cjk {
		inner := p["payload"].(map[string]any)
		inner["bill_to"] = "株式会社 田中商事"
		inner["items"] = []map[string]any{
			{"description": "ソフトウェアライセンス", "quantity": 5, "rate": 499.0},
		}
	}
	return p
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		fatal(err)
	}
	return b
}

func parseSteps(csv string) ([]int, error) {
	var steps []int
	for _, part := range strings.Split(csv, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid step %q", part)
		}
		steps = append(steps, n)
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("no steps given")
	}
	return steps, nil
}

func checkHealth(client *http.Client, baseURL string) error {
	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health status %d", resp.StatusCode)
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "stress:", err)
	os.Exit(1)
}
