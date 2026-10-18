package app

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/tim-riep/pdx/designer/internal/model"
)

// BenchmarkCommandPatch misst einen Befehl vom Aufruf bis zum Patch der
// Projektion: Anwenden, XSD- und Paketprüfung, Projektion und Diff. Ziel:
// p95 unter 20 ms bei der Musterrechnung.
func BenchmarkCommandPatch(b *testing.B) {
	c := NewCore(nil, nil, b.TempDir())
	if _, err := NewProjectService(c).Open(muster); err != nil {
		b.Fatal(err)
	}
	ts := NewTemplateService(c)
	times := make([]time.Duration, 0, b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		if _, err := ts.Apply("x", model.Command{Op: "setAttr", Ref: "adressfenster", Name: "x", Value: fmt.Sprintf("%dmm", 20+i%10)}); err != nil {
			b.Fatal(err)
		}
		times = append(times, time.Since(start))
	}
	b.StopTimer()
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	b.ReportMetric(float64(times[len(times)*95/100].Microseconds())/1000, "p95-ms")
}
