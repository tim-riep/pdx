package app

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/tim-riep/pdx/designer/internal/model"
)

// BenchmarkCommandPatch measures a command from the call to the patch of the
// projection: apply, XSD and package check, projection and diff. Goal:
// p95 below 20 ms for the sample invoice.
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
