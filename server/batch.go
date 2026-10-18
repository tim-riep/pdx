package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"
)

// batchResult ist der Eintrag eines Datensatzes in ergebnis.json.
type batchResult struct {
	Data     string    `json:"daten"`
	PDF      string    `json:"pdf,omitempty"`
	Pages    int       `json:"seiten,omitempty"`
	Findings []Finding `json:"befunde,omitempty"`
}

// batch rendert alle XML-Dateien eines ZIP mit derselben Vorlage:
// POST /v1/render/{id}/batch, Parameter wie beim Einzelrendern. Die Antwort
// ist ein ZIP als Stream mit einem PDF je Datensatz in Eingabereihenfolge
// und ergebnis.json am Ende. Da der Status mit dem ersten Byte feststeht,
// stehen Fehler einzelner Datensätze in ergebnis.json, alle auf einmal.
func (s *Server) batch(w http.ResponseWriter, r *http.Request) {
	opts, bad := s.renderOptions(r)
	if len(bad) > 0 {
		writeProblem(w, Problem{Status: http.StatusBadRequest, Title: "Ungültige Parameter", Findings: bad})
		return
	}
	raw, ok := readBody(w, r, s.cfg.MaxBatch)
	if !ok {
		return
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		writeProblem(w, Problem{Status: http.StatusBadRequest, Title: "Rumpf ist kein ZIP"})
		return
	}
	var files []*zip.File
	var problems []Finding
	names := map[string]bool{}
	for _, f := range zr.File {
		base := path.Base(f.Name)
		if f.FileInfo().IsDir() || strings.HasPrefix(f.Name, "__MACOSX/") || strings.HasPrefix(base, ".") || !strings.HasSuffix(strings.ToLower(base), ".xml") {
			continue
		}
		if names[base] {
			problems = append(problems, Finding{Message: fmt.Sprintf("Dateiname %s kommt mehrfach vor", base)})
		}
		names[base] = true
		if f.UncompressedSize64 > uint64(s.cfg.MaxBody) {
			problems = append(problems, Finding{Message: fmt.Sprintf("%s ist größer als %d Bytes", base, s.cfg.MaxBody)})
		}
		files = append(files, f)
	}
	if len(files) == 0 {
		problems = append(problems, Finding{Message: "das ZIP enthält keine XML-Datei"})
	}
	if len(files) > s.cfg.MaxBatchEntries {
		problems = append(problems, Finding{Message: fmt.Sprintf("%d Datensätze, höchstens %d sind erlaubt", len(files), s.cfg.MaxBatchEntries)})
	}
	if len(problems) > 0 {
		writeProblem(w, Problem{Status: http.StatusUnprocessableEntity, Title: "Stapel ungültig", Findings: problems})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.BatchTimeout.Duration)
	defer cancel()
	if !s.acquire(ctx) {
		writeProblem(w, Problem{Status: http.StatusServiceUnavailable, Title: "Ausgelastet"})
		return
	}
	q := r.URL.Query()
	c, ver, err := s.template(ctx, r.PathValue("id"), q.Get("version"), q.Get("lang"))
	s.release()
	if err != nil {
		templateProblem(w, err)
		return
	}
	opts.Fonts = c.fonts
	opts.Context = ctx

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("X-Pdx-Template-Version", ver)
	w.Header().Set("Cache-Control", "no-store")
	zw := zip.NewWriter(w)
	type out struct {
		pdf []byte
		res batchResult
	}
	// Je Runde so viele Datensätze wie Plätze, geschrieben in Reihenfolge.
	// So bleibt der Speicher begrenzt.
	window := cap(s.sem)
	var results []batchResult
	for start := 0; start < len(files); start += window {
		end := min(start+window, len(files))
		outs := make([]out, end-start)
		var wg sync.WaitGroup
		for i := start; i < end; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				f := files[i]
				o := &outs[i-start]
				o.res.Data = path.Base(f.Name)
				data, err := readEntry(f, s.cfg.MaxBody)
				if err != nil {
					o.res.Findings = []Finding{{Message: err.Error()}}
					return
				}
				if !s.acquire(ctx) {
					o.res.Findings = []Finding{{Message: "abgebrochen: Zeitlimit oder Auslastung"}}
					return
				}
				res, err := c.tpl.Render(data, opts)
				s.release()
				if err != nil {
					o.res.Findings = findings(err)
					return
				}
				o.pdf = res.PDF
				o.res.PDF = strings.TrimSuffix(o.res.Data, path.Ext(o.res.Data)) + ".pdf"
				o.res.Pages = len(res.Tree.Pages)
			}(i)
		}
		wg.Wait()
		for _, o := range outs {
			if o.pdf != nil {
				fw, err := zw.CreateHeader(&zip.FileHeader{Name: o.res.PDF, Method: zip.Deflate, Modified: opts.CreationDate})
				if err != nil {
					return
				}
				if _, err := fw.Write(o.pdf); err != nil {
					return
				}
				s.metrics.batch.Add(1)
			}
			results = append(results, o.res)
		}
		if f, ok := w.(http.Flusher); ok {
			zw.Flush()
			f.Flush()
		}
	}
	fw, err := zw.CreateHeader(&zip.FileHeader{Name: "ergebnis.json", Method: zip.Deflate, Modified: opts.CreationDate})
	if err == nil {
		enc := json.NewEncoder(fw)
		enc.SetIndent("", "  ")
		enc.Encode(map[string]any{"vorlage": r.PathValue("id"), "version": ver, "dokumente": results})
	}
	zw.Close()
}

// readEntry liest einen Eintrag höchstens bis max Bytes, auch wenn die
// Größe im Kopf falsch angegeben ist.
func readEntry(f *zip.File, max int64) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%s nicht lesbar: %v", f.Name, err)
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, max+1))
	if err != nil {
		return nil, fmt.Errorf("%s nicht lesbar: %v", f.Name, err)
	}
	if int64(len(b)) > max {
		return nil, fmt.Errorf("%s ist größer als %d Bytes", f.Name, max)
	}
	return b, nil
}
