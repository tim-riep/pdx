// Command last measures throughput and response times of pdx-server with
// fixed concurrency. It uses server.Client and no other dependency.
//
//	PDX_KEY=… go run ./deploy/last -url http://127.0.0.1:8080 -id rechnung \
//	    -data testdata/templates/musterrechnung/samples/rechnung.xml \
//	    -profile pdfa-3b -c 8 -d 30s
//
// The key comes from PDX_KEY, never as an argument. With -p95 and -rate the
// run fails (exit code 1) if the target values are missed. Agreed for the
// sample invoice with 8 concurrent requests on 4 CPUs: p95 below 300 ms
// and at least 30 PDFs per second.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/tim-riep/pdx/server"
)

func main() {
	base := flag.String("url", "http://127.0.0.1:8080", "Adresse von pdx-server")
	id := flag.String("id", "rechnung", "Vorlage")
	dataPath := flag.String("data", "", "Daten-XML")
	prof := flag.String("profile", "pdfa-3b", "Profil")
	conc := flag.Int("c", 8, "parallele Anfragen")
	dur := flag.Duration("d", 30*time.Second, "Dauer")
	warm := flag.Duration("warmup", 3*time.Second, "Aufwärmen vor der Messung")
	maxP95 := flag.Duration("p95", 0, "Zielwert: p95 höchstens so lang (0 ohne Prüfung)")
	minRate := flag.Float64("rate", 0, "Zielwert: mindestens so viele PDFs pro Sekunde (0 ohne Prüfung)")
	flag.Parse()
	data, err := os.ReadFile(*dataPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	c, err := server.NewClient(*base, os.Getenv("PDX_KEY"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	q := url.Values{"profile": {*prof}}
	run := func(d time.Duration) ([]time.Duration, int, int64) {
		var mu sync.Mutex
		var lat []time.Duration
		var errs int
		var bytes int64
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		var wg sync.WaitGroup
		for range *conc {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for ctx.Err() == nil {
					t0 := time.Now()
					pdf, err := c.Render(context.Background(), *id, data, q)
					el := time.Since(t0)
					mu.Lock()
					if err != nil {
						errs++
					} else {
						lat = append(lat, el)
						bytes += int64(len(pdf))
					}
					mu.Unlock()
				}
			}()
		}
		wg.Wait()
		return lat, errs, bytes
	}
	run(*warm)
	start := time.Now()
	lat, errs, bytes := run(*dur)
	total := time.Since(start)
	slices.Sort(lat)
	pct := func(p float64) time.Duration {
		if len(lat) == 0 {
			return 0
		}
		return lat[min(len(lat)-1, int(float64(len(lat))*p))].Round(time.Millisecond)
	}
	fmt.Printf("Profil %s, %d parallel, %s\n", *prof, *conc, total.Round(time.Second))
	fmt.Printf("PDFs: %d (%.1f/s), Fehler: %d, Größe im Mittel %d KB\n", len(lat), float64(len(lat))/total.Seconds(), errs, bytes/int64(max(1, len(lat)))/1024)
	fmt.Printf("p50 %v, p95 %v, p99 %v, max %v\n", pct(0.50), pct(0.95), pct(0.99), pct(1))
	rate := float64(len(lat)) / total.Seconds()
	var failed []string
	if errs > 0 {
		failed = append(failed, fmt.Sprintf("%d Fehler", errs))
	}
	if *maxP95 > 0 && pct(0.95) > *maxP95 {
		failed = append(failed, fmt.Sprintf("p95 %v über %v", pct(0.95), *maxP95))
	}
	if *minRate > 0 && rate < *minRate {
		failed = append(failed, fmt.Sprintf("%.1f PDFs/s unter %.1f", rate, *minRate))
	}
	if len(failed) > 0 {
		fmt.Println("Zielwerte verfehlt:", strings.Join(failed, ", "))
		os.Exit(1)
	}
	if *maxP95 > 0 || *minRate > 0 {
		fmt.Println("Zielwerte erreicht")
	}
}
