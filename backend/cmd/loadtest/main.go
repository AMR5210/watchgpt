package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/AMR5210/watchgpt/backend/internal/handler"
)

type headersFlag []string

func (h *headersFlag) String() string {
	return strings.Join(*h, ",")
}

func (h *headersFlag) Set(value string) error {
	*h = append(*h, value)
	return nil
}

func main() {
	var headers headersFlag
	url := flag.String("url", "", "URL to load test. Empty URL runs an in-process /health benchmark.")
	method := flag.String("method", http.MethodGet, "HTTP method")
	body := flag.String("body", "", "HTTP request body")
	requests := flag.Int("n", 10000, "total requests")
	concurrency := flag.Int("c", 50, "concurrent workers")
	flag.Var(&headers, "header", "HTTP header in 'Name: value' form. Repeatable.")
	flag.Parse()

	if *requests <= 0 || *concurrency <= 0 {
		panic("-n and -c must be positive")
	}

	results := run(*url, *method, *body, headers, *requests, *concurrency)
	printResults(results)
}

type result struct {
	durations []time.Duration
	statuses  map[int]int
	errors    int64
	elapsed   time.Duration
}

func run(url, method, body string, headers []string, requests, concurrency int) result {
	var (
		next      int64
		errors    int64
		statusMu  sync.Mutex
		statuses  = make(map[int]int)
		durations = make([]time.Duration, requests)
		wg        sync.WaitGroup
		start     = time.Now()
	)

	client := &http.Client{Timeout: 30 * time.Second}
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				index := int(atomic.AddInt64(&next, 1)) - 1
				if index >= requests {
					return
				}

				reqStart := time.Now()
				status, err := send(client, url, method, body, headers)
				durations[index] = time.Since(reqStart)
				if err != nil {
					atomic.AddInt64(&errors, 1)
					continue
				}

				statusMu.Lock()
				statuses[status]++
				statusMu.Unlock()
			}
		}()
	}
	wg.Wait()

	return result{
		durations: durations,
		statuses:  statuses,
		errors:    errors,
		elapsed:   time.Since(start),
	}
}

func send(client *http.Client, url, method, body string, headers []string) (int, error) {
	if url == "" {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		handler.Health(rec, req)
		return rec.Code, nil
	}

	req, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		return 0, err
	}
	for _, header := range headers {
		name, value, ok := strings.Cut(header, ":")
		if !ok {
			return 0, fmt.Errorf("invalid header %q", header)
		}
		req.Header.Set(strings.TrimSpace(name), strings.TrimSpace(value))
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func printResults(r result) {
	sort.Slice(r.durations, func(i, j int) bool {
		return r.durations[i] < r.durations[j]
	})

	total := len(r.durations)
	fmt.Printf("requests: %d\n", total)
	fmt.Printf("elapsed: %.3fs\n", r.elapsed.Seconds())
	fmt.Printf("throughput: %.1f req/s\n", float64(total)/r.elapsed.Seconds())
	fmt.Printf("errors: %d\n", r.errors)
	fmt.Printf("statuses: %v\n", r.statuses)
	fmt.Printf("min: %s\n", r.durations[0])
	fmt.Printf("p50: %s\n", percentile(r.durations, 50))
	fmt.Printf("p95: %s\n", percentile(r.durations, 95))
	fmt.Printf("p99: %s\n", percentile(r.durations, 99))
	fmt.Printf("max: %s\n", r.durations[len(r.durations)-1])
}

func percentile(values []time.Duration, p int) time.Duration {
	if len(values) == 0 {
		return 0
	}
	index := (len(values)*p + 99) / 100
	if index <= 0 {
		index = 1
	}
	if index > len(values) {
		index = len(values)
	}
	return values[index-1]
}
