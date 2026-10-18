// Command last misst Durchsatz und Antwortzeiten von pdx-server mit fester
// Parallelität. Es nutzt server.Client und keine weitere Abhängigkeit.
//
//	PDX_KEY=… go run ./deploy/last -url http://127.0.0.1:8080 -id rechnung \
//	    -data testdata/templates/musterrechnung/samples/rechnung.xml \
//	    -profile pdfa-3b -c 8 -d 30s
//
// Der Schlüssel kommt aus PDX_KEY, nie als Argument.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/url"
	"os"
	"slices"
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
}
