package server

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tim-riep/pdx"
	"github.com/tim-riep/pdx/font"
	"github.com/tim-riep/pdx/pdf/profile"
	"github.com/tim-riep/pdx/sign"
	"github.com/tim-riep/pdx/template/pkg"
	"github.com/tim-riep/pdx/verapdf"
)

// Server ist der Render-Dienst.
type Server struct {
	cfg     *Config
	store   Store
	log     *slog.Logger
	sem     chan struct{}
	cache   *lru
	group   flight
	metrics *metrics
	mux     *http.ServeMux
	// Verifier prüft mit veraPDF, nil schaltet /v1/verify ab.
	Verifier verapdf.Runner
	// Signer und TSA signieren für /v1/sign, ohne Signer ist es aus.
	Signer sign.Signer
	TSA    *sign.TSA
	// now liefert die Zeit, in Tests fest.
	now func() time.Time
}

// New legt den Dienst an. log darf nil sein.
func New(cfg *Config, store Store, log *slog.Logger) (*Server, error) {
	if err := cfg.Check(); err != nil {
		return nil, err
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Server{cfg: cfg, store: store, log: log, sem: make(chan struct{}, cfg.MaxConcurrent),
		cache: newLRU(32), metrics: newMetrics(), now: time.Now}, nil
}

//go:embed openapi.json
var openAPI []byte

// route ist ein Muster des Routings mit seinem Handler.
type route struct {
	pattern string
	h       http.Handler
}

// routes sind alle Routen, die Spezifikation in openapi.json beschreibt
// genau diese (Test).
func (s *Server) routes() []route {
	return []route{
		{"GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			io.WriteString(w, "ok\n")
		})},
		{"GET /readyz", http.HandlerFunc(s.ready)},
		{"GET /metrics", http.HandlerFunc(s.serveMetrics)},
		{"GET /openapi.json", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write(openAPI)
		})},
		{"POST /v1/render/{id}", s.auth(RightRender, s.render)},
		{"POST /v1/render/{id}/batch", s.auth(RightRender, s.batch)},
		{"GET /v1/templates", s.auth(RightTemplates, s.listTemplates)},
		{"POST /v1/templates/{id}", s.auth(RightTemplates, s.uploadTemplate)},
		{"GET /v1/templates/{id}/versions", s.auth(RightTemplates, s.listVersions)},
		{"GET /v1/templates/{id}/versions/{version}", s.auth(RightTemplates, s.getVersion)},
		{"PUT /v1/templates/{id}/active", s.auth(RightTemplates, s.activate)},
		{"POST /v1/verify", s.auth(RightVerify, s.verify)},
		{"POST /v1/sign", s.auth(RightSign, s.sign)},
	}
}

// Handler liefert die Routen des Dienstes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	for _, rt := range s.routes() {
		mux.Handle(rt.pattern, rt.h)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, Problem{Status: http.StatusNotFound, Title: "Pfad unbekannt"})
	})
	s.mux = mux
	return s.logRequests(mux)
}

// ready meldet Bereitschaft, wenn die Ablage lesbar ist.
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if _, err := s.store.List(r.Context()); err != nil {
		writeProblem(w, Problem{Status: http.StatusServiceUnavailable, Title: "Ablage nicht verfügbar"})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	io.WriteString(w, "bereit\n")
}

type keyCtx struct{}

// auth verlangt einen API-Schlüssel mit dem Recht right, als
// "Authorization: Bearer" oder "X-API-Key".
func (s *Server) auth(right string, h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("X-API-Key")
		if v, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
			key = v
		}
		k := s.cfg.lookup(key)
		if key == "" || k == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="pdx"`)
			writeProblem(w, Problem{Status: http.StatusUnauthorized, Title: "API-Schlüssel fehlt oder ist unbekannt"})
			return
		}
		if rec, ok := w.(*recorder); ok {
			rec.key = k.Name
		}
		if !slices.Contains(k.Rights, right) {
			writeProblem(w, Problem{Status: http.StatusForbidden, Title: "Recht fehlt", Detail: "Der Schlüssel hat das Recht " + right + " nicht."})
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), keyCtx{}, k)))
	})
}

// recorder merkt sich Status und Größe für das Log.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	key    string
}

func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

func (r *recorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

// logRequests protokolliert je Anfrage Methode, Muster der Route, Status,
// Dauer, Größe und Namen des Schlüssels. Nie Rumpf, Abfrage oder Werte
// aus dem Pfad, so kommen keine Nutzdaten ins Log.
func (s *Server) logRequests(h *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w}
		s.metrics.inFlight.Add(1)
		h.ServeHTTP(rec, r)
		s.metrics.inFlight.Add(-1)
		_, pattern := h.Handler(r)
		s.metrics.observe(pattern, rec.status, time.Since(start))
		s.log.LogAttrs(r.Context(), slog.LevelInfo, "anfrage",
			slog.String("methode", r.Method),
			slog.String("route", pattern),
			slog.Int("status", rec.status),
			slog.Duration("dauer", time.Since(start)),
			slog.Int64("bytes", rec.bytes),
			slog.String("schluessel", rec.key),
		)
	})
}

// acquire wartet auf einen freien Platz, höchstens QueueWait.
func (s *Server) acquire(ctx context.Context) bool {
	t := time.NewTimer(s.cfg.QueueWait.Duration)
	defer t.Stop()
	select {
	case s.sem <- struct{}{}:
		return true
	case <-t.C:
		return false
	case <-ctx.Done():
		return false
	}
}

func (s *Server) release() { <-s.sem }

// template liefert die kompilierte Vorlage aus dem Cache oder der Ablage.
// Gleichzeitige Anfragen kompilieren dieselbe Version nur einmal.
func (s *Server) template(ctx context.Context, id, version, lang string) (*compiled, string, error) {
	raw, ver, err := s.store.Get(ctx, id, version)
	if err != nil {
		return nil, "", err
	}
	key := id + "\x00" + ver + "\x00" + lang
	if c, ok := s.cache.get(key); ok {
		return c, ver, nil
	}
	v, err, _ := s.group.Do(key, func() (any, error) {
		p, err := pkg.Read(raw)
		if err != nil {
			return nil, err
		}
		t, err := pdx.CompileTemplate(p, lang)
		if err != nil {
			return nil, err
		}
		c := &compiled{tpl: t, fonts: font.NewRegistry()}
		s.cache.put(key, c)
		return c, nil
	})
	if err != nil {
		return nil, "", err
	}
	return v.(*compiled), ver, nil
}

// renderOptions liest die Parameter zum Rendern, alle Befunde auf einmal.
func (s *Server) renderOptions(r *http.Request) (pdx.TemplateOptions, []Finding) {
	q := r.URL.Query()
	opts := pdx.TemplateOptions{Strict: q.Get("strict") == "true"}
	var bad []Finding
	if p := q.Get("profile"); p != "" {
		if _, err := profile.Lookup(p); err != nil {
			bad = append(bad, Finding{Message: err.Error()})
		}
		opts.Profile = p
	}
	opts.CreationDate = s.now().UTC().Truncate(time.Second)
	if d := q.Get("creationDate"); d != "" {
		t, err := time.Parse(time.RFC3339, d)
		if err != nil {
			bad = append(bad, Finding{Message: "creationDate ist kein Zeitpunkt nach RFC 3339"})
		}
		opts.CreationDate = t
	}
	return opts, bad
}

// readBody liest den Rumpf bis max Bytes und meldet Fehler als Problem.
func readBody(w http.ResponseWriter, r *http.Request, max int64) ([]byte, bool) {
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, max))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeProblem(w, Problem{Status: http.StatusRequestEntityTooLarge, Title: "Anfrage zu groß",
				Detail: fmt.Sprintf("Höchstens %d Bytes sind erlaubt.", max)})
			return nil, false
		}
		writeProblem(w, Problem{Status: http.StatusBadRequest, Title: "Rumpf nicht lesbar"})
		return nil, false
	}
	return data, true
}

// templateProblem meldet einen Fehler beim Laden der Vorlage.
func templateProblem(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		writeProblem(w, Problem{Status: http.StatusNotFound, Title: "Vorlage oder Version unbekannt"})
		return
	}
	writeProblem(w, Problem{Type: typeBase + "vorlage", Status: http.StatusUnprocessableEntity, Title: "Vorlage fehlerhaft", Findings: findings(err)})
}

// render setzt einen Datensatz: POST /v1/render/{id}?version=&profile=
// &lang=&strict=&creationDate=, Daten-XML im Rumpf, PDF als Antwort.
func (s *Server) render(w http.ResponseWriter, r *http.Request) {
	opts, bad := s.renderOptions(r)
	if len(bad) > 0 {
		writeProblem(w, Problem{Status: http.StatusBadRequest, Title: "Ungültige Parameter", Findings: bad})
		return
	}
	data, ok := readBody(w, r, s.cfg.MaxBody)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timeout.Duration)
	defer cancel()
	if !s.acquire(ctx) {
		writeProblem(w, Problem{Status: http.StatusServiceUnavailable, Title: "Ausgelastet", Detail: "Alle Plätze belegt, bitte später erneut."})
		return
	}
	defer s.release()
	q := r.URL.Query()
	c, ver, err := s.template(ctx, r.PathValue("id"), q.Get("version"), q.Get("lang"))
	if err != nil {
		templateProblem(w, err)
		return
	}
	opts.Fonts = c.fonts
	opts.Context = ctx
	res, err := c.tpl.Render(data, opts)
	if err != nil {
		if ctx.Err() != nil {
			writeProblem(w, Problem{Status: http.StatusGatewayTimeout, Title: "Zeitlimit überschritten"})
			return
		}
		writeProblem(w, Problem{Type: typeBase + "rendern", Status: http.StatusUnprocessableEntity, Title: "Rendern fehlgeschlagen", Findings: findings(err)})
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Length", strconv.Itoa(len(res.PDF)))
	w.Header().Set("X-Pdx-Template-Version", ver)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(res.PDF)
}
