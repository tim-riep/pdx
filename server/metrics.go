package server

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// buckets are the bounds of the duration histogram in seconds.
var buckets = []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// metrics counts requests in the Prometheus text format (Exposition Format
// 0.0.4). Labels are only route and status, never payload data or IDs.
type metrics struct {
	mu       sync.Mutex
	requests map[[2]string]uint64 // route, status
	hist     map[string]*histogram
	inFlight atomic.Int64
	batch    atomic.Uint64
	start    time.Time
}

type histogram struct {
	counts []uint64
	sum    float64
	n      uint64
}

func newMetrics() *metrics {
	return &metrics{requests: map[[2]string]uint64{}, hist: map[string]*histogram{}, start: time.Now()}
}

func (m *metrics) observe(route string, status int, d time.Duration) {
	if route == "" {
		route = "unbekannt"
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[[2]string{route, strconv.Itoa(status)}]++
	h := m.hist[route]
	if h == nil {
		h = &histogram{counts: make([]uint64, len(buckets))}
		m.hist[route] = h
	}
	sec := d.Seconds()
	for i, b := range buckets {
		if sec <= b {
			h.counts[i]++
		}
	}
	h.sum += sec
	h.n++
}

func label(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(s)
}

func (m *metrics) write(w io.Writer, s *Server) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fmt.Fprintln(w, "# HELP pdx_requests_total Anfragen je Route und Status.")
	fmt.Fprintln(w, "# TYPE pdx_requests_total counter")
	keys := make([][2]string, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i][0] < keys[j][0] || keys[i][0] == keys[j][0] && keys[i][1] < keys[j][1]
	})
	for _, k := range keys {
		fmt.Fprintf(w, "pdx_requests_total{route=\"%s\",status=\"%s\"} %d\n", label(k[0]), k[1], m.requests[k])
	}
	fmt.Fprintln(w, "# HELP pdx_request_duration_seconds Dauer der Anfragen je Route.")
	fmt.Fprintln(w, "# TYPE pdx_request_duration_seconds histogram")
	routes := make([]string, 0, len(m.hist))
	for r := range m.hist {
		routes = append(routes, r)
	}
	sort.Strings(routes)
	for _, r := range routes {
		h := m.hist[r]
		for i, b := range buckets {
			fmt.Fprintf(w, "pdx_request_duration_seconds_bucket{route=\"%s\",le=\"%s\"} %d\n", label(r), strconv.FormatFloat(b, 'g', -1, 64), h.counts[i])
		}
		fmt.Fprintf(w, "pdx_request_duration_seconds_bucket{route=\"%s\",le=\"+Inf\"} %d\n", label(r), h.n)
		fmt.Fprintf(w, "pdx_request_duration_seconds_sum{route=\"%s\"} %s\n", label(r), strconv.FormatFloat(h.sum, 'g', -1, 64))
		fmt.Fprintf(w, "pdx_request_duration_seconds_count{route=\"%s\"} %d\n", label(r), h.n)
	}
	fmt.Fprintln(w, "# HELP pdx_in_flight Laufende Anfragen.")
	fmt.Fprintln(w, "# TYPE pdx_in_flight gauge")
	fmt.Fprintf(w, "pdx_in_flight %d\n", m.inFlight.Load())
	fmt.Fprintln(w, "# HELP pdx_render_slots_busy Belegte Plätze zum Rendern.")
	fmt.Fprintln(w, "# TYPE pdx_render_slots_busy gauge")
	fmt.Fprintf(w, "pdx_render_slots_busy %d\n", len(s.sem))
	fmt.Fprintln(w, "# HELP pdx_render_slots Plätze zum Rendern insgesamt.")
	fmt.Fprintln(w, "# TYPE pdx_render_slots gauge")
	fmt.Fprintf(w, "pdx_render_slots %d\n", cap(s.sem))
	fmt.Fprintln(w, "# HELP pdx_template_cache_entries Kompilierte Vorlagen im Cache.")
	fmt.Fprintln(w, "# TYPE pdx_template_cache_entries gauge")
	fmt.Fprintf(w, "pdx_template_cache_entries %d\n", s.cache.len())
	fmt.Fprintln(w, "# HELP pdx_batch_documents_total In Stapeln gerenderte Dokumente.")
	fmt.Fprintln(w, "# TYPE pdx_batch_documents_total counter")
	fmt.Fprintf(w, "pdx_batch_documents_total %d\n", m.batch.Load())
	fmt.Fprintln(w, "# HELP pdx_uptime_seconds Laufzeit des Dienstes.")
	fmt.Fprintln(w, "# TYPE pdx_uptime_seconds gauge")
	fmt.Fprintf(w, "pdx_uptime_seconds %s\n", strconv.FormatFloat(time.Since(m.start).Seconds(), 'f', 0, 64))
}

func (s *Server) serveMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	s.metrics.write(w, s)
}
