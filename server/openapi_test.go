package server

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// spec ist die geladene Spezifikation, generisch als JSON.
var spec = sync.OnceValue(func() map[string]any {
	var m map[string]any
	if err := json.Unmarshal(openAPI, &m); err != nil {
		panic(err)
	}
	return m
})

// resolve folgt $ref innerhalb der Spezifikation.
func resolve(v any) map[string]any {
	m, _ := v.(map[string]any)
	for m != nil {
		ref, ok := m["$ref"].(string)
		if !ok {
			return m
		}
		var cur any = spec()
		for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
			cur = cur.(map[string]any)[part]
		}
		m, _ = cur.(map[string]any)
	}
	return m
}

// validate prüft einen JSON-Wert gegen eine Teilmenge von JSON Schema:
// type, properties, required, items, enum, pattern und $ref. Alle
// Abweichungen auf einmal.
func validate(schema map[string]any, v any, path string) []string {
	schema = resolve(schema)
	if schema == nil {
		return nil
	}
	var errs []string
	if enum, ok := schema["enum"].([]any); ok && !slices.Contains(enum, v) {
		errs = append(errs, fmt.Sprintf("%s: %v nicht in %v", path, v, enum))
	}
	switch schema["type"] {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			return append(errs, path+": kein Objekt")
		}
		for _, r := range schema["required"].([]any) {
			if _, ok := obj[r.(string)]; !ok {
				errs = append(errs, fmt.Sprintf("%s: %s fehlt", path, r))
			}
		}
		props, _ := schema["properties"].(map[string]any)
		for k, val := range obj {
			ps, ok := props[k]
			if !ok && props != nil {
				errs = append(errs, fmt.Sprintf("%s: %s ist nicht beschrieben", path, k))
				continue
			}
			errs = append(errs, validate(resolve(ps), val, path+"."+k)...)
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			return append(errs, path+": kein Array")
		}
		for i, it := range arr {
			errs = append(errs, validate(resolve(schema["items"]), it, path+"["+strconv.Itoa(i)+"]")...)
		}
	case "string":
		s, ok := v.(string)
		if !ok {
			return append(errs, path+": kein String")
		}
		if p, ok := schema["pattern"].(string); ok && !regexp.MustCompile(p).MatchString(s) {
			errs = append(errs, path+": passt nicht zu "+p)
		}
	case "integer":
		if f, ok := v.(float64); !ok || f != float64(int64(f)) {
			errs = append(errs, path+": keine Ganzzahl")
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			errs = append(errs, path+": kein Wahrheitswert")
		}
	}
	return errs
}

// operation liefert die Operation zu einem Muster wie "POST /v1/render/{id}".
func operation(pattern string) map[string]any {
	method, path, _ := strings.Cut(pattern, " ")
	return resolve(resolve(spec()["paths"].(map[string]any)[path])[strings.ToLower(method)])
}

// checkResponse prüft eine Antwort gegen die Spezifikation.
func checkResponse(pattern string, status int, header http.Header, body []byte) []string {
	op := operation(pattern)
	if op == nil {
		return []string{pattern + " ist nicht beschrieben"}
	}
	resp := resolve(op["responses"].(map[string]any)[strconv.Itoa(status)])
	if resp == nil {
		return []string{fmt.Sprintf("%s: Status %d ist nicht beschrieben", pattern, status)}
	}
	content, _ := resp["content"].(map[string]any)
	if content == nil {
		return nil
	}
	ct, _, _ := mime.ParseMediaType(header.Get("Content-Type"))
	media := resolve(content[ct])
	if media == nil {
		return []string{fmt.Sprintf("%s %d: Content-Type %q ist nicht beschrieben", pattern, status, ct)}
	}
	if ct != "application/json" && ct != "application/problem+json" {
		return nil
	}
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		return []string{fmt.Sprintf("%s %d: kein JSON", pattern, status)}
	}
	var errs []string
	for _, e := range validate(resolve(media["schema"]), v, "$") {
		errs = append(errs, fmt.Sprintf("%s %d: %s", pattern, status, e))
	}
	return errs
}

// specHandler prüft jede Antwort des Dienstes in Tests gegen die
// Spezifikation. Alle Tests mit newTest laufen darüber.
func specHandler(t *testing.T, s *Server) http.Handler {
	h := s.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if _, pattern := s.mux.Handler(r); pattern != "/" {
			for _, e := range checkResponse(pattern, rec.Code, rec.Header(), rec.Body.Bytes()) {
				t.Errorf("OpenAPI: %s", e)
			}
		}
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		w.Write(rec.Body.Bytes())
	})
}

// Jede Route ist beschrieben und jede beschriebene Operation gibt es.
func TestOpenAPIRoutes(t *testing.T) {
	s, _ := New(testConfig(), &MemStore{}, nil)
	var code, doc []string
	for _, rt := range s.routes() {
		code = append(code, rt.pattern)
	}
	for path, item := range spec()["paths"].(map[string]any) {
		for m := range item.(map[string]any) {
			doc = append(doc, strings.ToUpper(m)+" "+path)
		}
	}
	sort.Strings(code)
	sort.Strings(doc)
	if strings.Join(code, "\n") != strings.Join(doc, "\n") {
		t.Errorf("Routen im Code:\n%s\n\nin openapi.json:\n%s", strings.Join(code, "\n"), strings.Join(doc, "\n"))
	}
	if spec()["openapi"] != "3.1.0" {
		t.Error("keine OpenAPI 3.1")
	}
	// Jeder $ref zeigt auf etwas.
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if ref, ok := x["$ref"].(string); ok && resolve(x) == nil {
				t.Errorf("$ref %s ins Leere", ref)
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(spec())
}

// Der Validator selbst erkennt Abweichungen.
func TestOpenAPIValidator(t *testing.T) {
	errs := checkResponse("POST /v1/templates/{id}", 201, http.Header{"Content-Type": {"application/json"}},
		[]byte(`{"version":1,"sha256":"x","size":1.5,"created":"t","active":"ja","extra":true}`))
	if len(errs) != 4 {
		t.Errorf("%d Befunde: %v", len(errs), errs)
	}
	if errs := checkResponse("POST /v1/render/{id}", 418, http.Header{}, nil); len(errs) != 1 {
		t.Error("unbeschriebener Status nicht erkannt")
	}
	if errs := checkResponse("POST /v1/render/{id}", 200, http.Header{"Content-Type": {"text/html"}}, nil); len(errs) != 1 {
		t.Error("falscher Content-Type nicht erkannt")
	}
}
