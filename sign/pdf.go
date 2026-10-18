package sign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ocsp"

	"github.com/tim-riep/pdx/pdf"
	"github.com/tim-riep/pdx/pdf/incr"
	"github.com/tim-riep/pdx/pdf/reader"
)

// Level ist eine Stufe von PAdES baseline (ETSI EN 319 142-1 6.1).
type Level string

const (
	LevelB   Level = "B-B"
	LevelT   Level = "B-T"
	LevelLT  Level = "B-LT"
	LevelLTA Level = "B-LTA"
)

var levelRank = map[Level]int{LevelB: 0, LevelT: 1, LevelLT: 2, LevelLTA: 3}

// PDFOptions steuert das Signieren eines PDF.
type PDFOptions struct {
	Level Level
	// TSA liefert Zeitstempel, Pflicht ab B-T.
	TSA *TSA
	// Field ist der Name eines vorhandenen, unsignierten Signaturfelds.
	// Leer legt pdx ein unsichtbares Feld an.
	Field string
	// Angaben im Signaturdictionary (ISO 32000-1 12.8.1).
	Reason, Location, ContactInfo, Name string
	// Time ist die behauptete Signaturzeit für /M, Vorgabe jetzt.
	Time time.Time
	// Reserve ist der Platz für den CMS-Container in Bytes, Vorgabe 16 KiB.
	Reserve int
	// Client holt Zertifikate, OCSP-Antworten und CRLs für B-LT, Vorgabe
	// mit 30 Sekunden Zeitlimit.
	Client *http.Client
}

// SignPDF signiert ein fertiges PDF nach PAdES baseline in der gewählten
// Stufe, ausschließlich per inkrementellem Update (ISO 32000-1 7.5.6), damit
// PDF/A-Konformität und frühere Signaturen erhalten bleiben.
//
// Signaturdictionary nach ETSI EN 319 142-1 6.3: Filter Adobe.PPKLite,
// SubFilter ETSI.CAdES.detached, ByteRange über die ganze Datei außer
// Contents, Zeit in M, kein Cert. B-T fügt einen Zeitstempel über die
// Signatur als unsigniertes Attribut hinzu, B-LT einen DSS mit Zertifikaten,
// OCSP-Antworten und CRLs (5.4.2), B-LTA danach einen Dokument-Zeitstempel
// (5.4.3) und erneut den DSS.
func SignPDF(ctx context.Context, in []byte, s Signer, opt PDFOptions) ([]byte, error) {
	if opt.Level == "" {
		opt.Level = LevelB
	}
	rank, ok := levelRank[opt.Level]
	if !ok {
		return nil, fmt.Errorf("Stufe %q ist unbekannt, erlaubt sind B-B, B-T, B-LT, B-LTA", opt.Level)
	}
	if rank >= 1 && opt.TSA == nil {
		return nil, fmt.Errorf("Stufe %s braucht einen Zeitstempeldienst (--tsa)", opt.Level)
	}
	if opt.Time.IsZero() {
		opt.Time = time.Now()
	}
	if opt.Reserve == 0 {
		opt.Reserve = 16 << 10
	}
	if opt.Client == nil {
		opt.Client = &http.Client{Timeout: 30 * time.Second}
	}
	certs := s.Certificates()
	if len(certs) == 0 {
		return nil, errors.New("der Signer hat kein Zertifikat")
	}
	if err := checkValidity(certs, opt.Time); err != nil {
		return nil, err
	}
	var tsTokens [][]byte
	sig := pdf.D(
		"Type", pdf.Name("Sig"),
		"Filter", pdf.Name("Adobe.PPKLite"),
		"SubFilter", pdf.Name("ETSI.CAdES.detached"),
		"M", pdf.Date(opt.Time),
	)
	for _, kv := range [][2]string{{"Reason", opt.Reason}, {"Location", opt.Location}, {"ContactInfo", opt.ContactInfo}, {"Name", opt.Name}} {
		if kv[1] != "" {
			sig.Set(pdf.Name(kv[0]), pdf.TextString(kv[1]))
		}
	}
	name := opt.Name
	if name == "" {
		name = certs[0].Subject.CommonName
	}
	lines := []string{"Digital signiert von " + name, "Datum: " + opt.Time.UTC().Format("2006-01-02 15:04 MST")}
	if opt.Reason != "" {
		lines = append(lines, "Grund: "+opt.Reason)
	}
	if opt.Location != "" {
		lines = append(lines, "Ort: "+opt.Location)
	}
	out, err := signRevision(in, opt.Field, "Signatur", sig, lines, opt.Reserve, func(content []byte) ([]byte, error) {
		so := SignedOptions{Detached: true}
		if rank >= 1 {
			so.Unsigned = func(signature []byte) ([][]byte, error) {
				tok, err := opt.TSA.Timestamp(ctx, signature)
				if err != nil {
					return nil, err
				}
				tsTokens = append(tsTokens, tok)
				return [][]byte{TimeStampAttribute(tok)}, nil
			}
		}
		return SignedData(ctx, s, content, so)
	})
	if err != nil || rank < 2 {
		return out, err
	}
	if out, err = addDSS(ctx, out, certs, tsTokens, opt); err != nil {
		return nil, err
	}
	if rank < 3 {
		return out, nil
	}
	docTS := pdf.D(
		"Type", pdf.Name("DocTimeStamp"),
		"Filter", pdf.Name("Adobe.PPKLite"),
		"SubFilter", pdf.Name("ETSI.RFC3161"),
	)
	var docToken []byte
	out, err = signRevision(out, "", "Zeitstempel", docTS, nil, 8<<10, func(content []byte) ([]byte, error) {
		tok, err := opt.TSA.Timestamp(ctx, content)
		docToken = tok
		return tok, err
	})
	if err != nil {
		return nil, err
	}
	return addDSS(ctx, out, nil, [][]byte{docToken}, opt)
}

// checkValidity lehnt Zertifikate ab, die zur Signaturzeit noch nicht oder
// nicht mehr gelten.
func checkValidity(certs []*x509.Certificate, at time.Time) error {
	var errs []error
	for _, c := range certs {
		switch {
		case at.After(c.NotAfter):
			errs = append(errs, fmt.Errorf("Zertifikat %q ist seit %s abgelaufen", c.Subject.CommonName, c.NotAfter.Format(time.DateOnly)))
		case at.Before(c.NotBefore):
			errs = append(errs, fmt.Errorf("Zertifikat %q gilt erst ab %s", c.Subject.CommonName, c.NotBefore.Format(time.DateOnly)))
		}
	}
	return errors.Join(errs...)
}

// byteRangeMark ist der Platzhalter der ByteRange mit fester Breite.
var byteRangeMark = pdf.Array{pdf.Int(0), pdf.Int(1111111111), pdf.Int(2222222222), pdf.Int(3333333333)}

// signRevision hängt ein Signaturdictionary an ein Signaturfeld und setzt
// nach dem Schreiben ByteRange und Contents ein. contents liefert aus den
// Bytes der ByteRange den Inhalt von Contents (CMS oder Zeitstempel). Ein
// sichtbares Feld aus der Vorlage zeigt danach lines.
func signRevision(in []byte, fieldName, prefix string, sig *pdf.Dict, lines []string, reserve int, contents func([]byte) ([]byte, error)) ([]byte, error) {
	u, err := incr.Open(in)
	if err != nil {
		return nil, err
	}
	f := u.File()
	cat, err := u.Dict(u.Root())
	if err != nil {
		return nil, err
	}
	acro, acroRef, err := acroForm(u, cat)
	if err != nil {
		return nil, err
	}
	fields, _ := acro.Get("Fields").(pdf.Array)
	var fieldRef pdf.Ref
	var field *pdf.Dict
	if fieldName != "" {
		r, ok := findField(f, f.Root()["AcroForm"], fieldName)
		if !ok {
			return nil, fmt.Errorf("Signaturfeld %q fehlt im PDF", fieldName)
		}
		fieldRef = pdf.Ref{Num: r.Num}
		if field, err = u.Dict(fieldRef); err != nil {
			return nil, err
		}
		if ft, _ := field.Get("FT").(pdf.Name); ft != "Sig" {
			return nil, fmt.Errorf("Feld %q ist kein Signaturfeld", fieldName)
		}
		if field.Has("V") {
			return nil, fmt.Errorf("Signaturfeld %q ist bereits signiert", fieldName)
		}
		// Feld und Widget sind bei pdx ein Objekt. Das neue Bild nennt
		// Unterzeichner und Zeit, Contents trägt denselben Text für
		// Hilfstechnik (ISO 32000-1 12.5.2, ISO 14289-2 8.10.2.3).
		if ap := signatureAppearance(f, f.Dict(r), lines); ap != nil && !field.Has("Kids") {
			field.Set("AP", pdf.D("N", u.Add(ap)))
			field.Set("Contents", pdf.TextString(strings.Join(lines, ", ")))
		}
	} else {
		pages, pageRefs := f.Pages()
		if len(pages) == 0 {
			return nil, errors.New("das PDF hat keine Seiten")
		}
		page := pdf.Ref{Num: pageRefs[0].Num}
		fieldRef = u.Reserve()
		// Unsichtbares Widget ohne Fläche: kein Erscheinungsbild nötig
		// (ISO 19005-2 6.3.3), F mit Print (6.3.2).
		field = pdf.D(
			"FT", pdf.Name("Sig"),
			"T", pdf.TextString(uniqueName(f, prefix)),
			"Type", pdf.Name("Annot"),
			"Subtype", pdf.Name("Widget"),
			"Rect", pdf.Array{pdf.Int(0), pdf.Int(0), pdf.Int(0), pdf.Int(0)},
			"F", pdf.Int(4),
			"P", page,
		)
		fields = append(fields, fieldRef)
		if err := addAnnot(u, page, fieldRef); err != nil {
			return nil, err
		}
	}
	sigRef := u.Reserve()
	sig.Set("ByteRange", byteRangeMark)
	sig.Set("Contents", pdf.HexString(make([]byte, reserve)))
	u.Set(sigRef, sig)
	field.Set("V", sigRef)
	u.Set(fieldRef, field)
	acro.Set("Fields", fields)
	// SignaturesExist und AppendOnly (ISO 32000-1 Tabelle 219).
	acro.Set("SigFlags", pdf.Int(3))
	if acroRef.Num != 0 {
		u.Set(acroRef, acro)
	} else {
		cat.Set("AcroForm", acro)
	}
	u.Set(u.Root(), cat)
	res, err := u.Bytes()
	if err != nil {
		return nil, err
	}
	data := res.Data
	start := res.Offsets[sigRef.Num]
	mark := []byte("/ByteRange " + pdf.Serialize(byteRangeMark))
	bi := bytes.Index(data[start:], mark)
	ci := bytes.Index(data[start:], []byte("/Contents <"))
	if bi < 0 || ci < 0 {
		return nil, errors.New("Platzhalter der Signatur nicht gefunden")
	}
	bi += start + len("/ByteRange ")
	cStart := start + ci + len("/Contents ")
	cEnd := cStart + 2 + 2*reserve
	if data[cEnd-1] != '>' {
		return nil, errors.New("Platzhalter für Contents hat nicht die erwartete Länge")
	}
	br := fmt.Sprintf("[0 %d %d %d", cStart, cEnd, len(data)-cEnd)
	width := len(pdf.Serialize(byteRangeMark))
	copy(data[bi:], br+strings.Repeat(" ", width-len(br)-1)+"]")
	signed := append(append([]byte(nil), data[:cStart]...), data[cEnd:]...)
	c, err := contents(signed)
	if err != nil {
		return nil, err
	}
	hx := strings.ToUpper(hex.EncodeToString(c))
	if len(hx) > 2*reserve {
		return nil, fmt.Errorf("Signatur braucht %d Bytes, reserviert sind %d", len(c), reserve)
	}
	copy(data[cStart+1:], hx)
	return data, nil
}

// acroForm liefert das AcroForm-Dictionary zum Ändern und seinen Verweis,
// wenn es ein eigenes Objekt ist.
func acroForm(u *incr.Update, cat *pdf.Dict) (*pdf.Dict, pdf.Ref, error) {
	switch v := cat.Get("AcroForm").(type) {
	case pdf.Ref:
		d, err := u.Dict(v)
		if err != nil {
			return nil, pdf.Ref{}, err
		}
		if fr, ok := d.Get("Fields").(pdf.Ref); ok {
			// Fields als eigenes Objekt wird zum direkten Array.
			o, err := u.Object(fr)
			if err != nil {
				return nil, pdf.Ref{}, err
			}
			d.Set("Fields", o)
		}
		return d, v, nil
	case *pdf.Dict:
		return v, pdf.Ref{}, nil
	case nil:
		return pdf.D("Fields", pdf.Array{}), pdf.Ref{}, nil
	}
	return nil, pdf.Ref{}, errors.New("AcroForm hat einen unerwarteten Typ")
}

// findField sucht ein Feld nach vollem Namen.
func findField(f *reader.File, acro reader.Object, name string) (reader.Ref, bool) {
	var found reader.Ref
	var walk func(o reader.Object, prefix string, depth int) bool
	walk = func(o reader.Object, prefix string, depth int) bool {
		d := f.Dict(o)
		if d == nil || depth > 32 {
			return false
		}
		full := prefix
		if t, ok := f.Resolve(d["T"]).(reader.String); ok {
			if full != "" {
				full += "."
			}
			full += reader.TextString(t)
		}
		if full == name {
			if r, ok := o.(reader.Ref); ok {
				found = r
				return true
			}
		}
		for _, k := range f.Array(d["Kids"]) {
			if walk(k, full, depth+1) {
				return true
			}
		}
		return false
	}
	for _, o := range f.Array(f.Dict(acro)["Fields"]) {
		if walk(o, "", 0) {
			return found, true
		}
	}
	return reader.Ref{}, false
}

// uniqueName liefert einen freien Feldnamen wie Signatur1.
func uniqueName(f *reader.File, prefix string) string {
	used := map[string]bool{}
	for _, fl := range f.Fields() {
		used[fl.Name] = true
	}
	for i := 1; ; i++ {
		n := prefix + strconv.Itoa(i)
		if !used[n] {
			return n
		}
	}
}

// addAnnot hängt eine Annotation an die Seite.
func addAnnot(u *incr.Update, page, annot pdf.Ref) error {
	pd, err := u.Dict(page)
	if err != nil {
		return err
	}
	switch v := pd.Get("Annots").(type) {
	case pdf.Ref:
		o, err := u.Object(v)
		if err != nil {
			return err
		}
		arr, _ := o.(pdf.Array)
		u.Set(v, append(arr, annot))
		return nil
	case pdf.Array:
		pd.Set("Annots", append(v, annot))
	default:
		pd.Set("Annots", pdf.Array{annot})
	}
	u.Set(page, pd)
	return nil
}

// validation sammelt Zertifikate und Sperrinformationen für den DSS.
type validation struct {
	opt   PDFOptions
	ctx   context.Context
	certs []*x509.Certificate
	ocsps [][]byte
	crls  [][]byte
	seen  map[string]bool
	errs  []error
}

// addDSS hängt einen DSS mit allem an, was die Prüfung der Signatur und
// der Zeitstempel braucht: Zertifikate bis zur Wurzel, OCSP-Antworten und
// CRLs (ETSI EN 319 142-1 6.3 r, s). Ein vorhandener DSS wird übernommen
// (5.4.1).
func addDSS(ctx context.Context, in []byte, certs []*x509.Certificate, tokens [][]byte, opt PDFOptions) ([]byte, error) {
	v := &validation{opt: opt, ctx: ctx, seen: map[string]bool{}}
	v.chain(certs)
	for _, tok := range tokens {
		info, err := ParseTimeStamp(tok)
		if err != nil {
			return nil, err
		}
		if err := checkValidity(info.Certificates[:min(1, len(info.Certificates))], time.Now()); err != nil {
			return nil, fmt.Errorf("Zeitstempeldienst: %w", err)
		}
		v.chain(info.Certificates)
	}
	if len(v.errs) > 0 {
		return nil, errors.Join(v.errs...)
	}
	u, err := incr.Open(in)
	if err != nil {
		return nil, err
	}
	cat, err := u.Dict(u.Root())
	if err != nil {
		return nil, err
	}
	dss := pdf.D("Type", pdf.Name("DSS"))
	var oldCerts, oldOCSPs, oldCRLs pdf.Array
	if r, ok := cat.Get("DSS").(pdf.Ref); ok {
		if old, err := u.Dict(r); err == nil {
			oldCerts, _ = old.Get("Certs").(pdf.Array)
			oldOCSPs, _ = old.Get("OCSPs").(pdf.Array)
			oldCRLs, _ = old.Get("CRLs").(pdf.Array)
		}
	}
	streams := func(old pdf.Array, items [][]byte) pdf.Array {
		out := append(pdf.Array(nil), old...)
		for _, b := range items {
			out = append(out, u.Add(pdf.NewFlateStream(nil, b)))
		}
		return out
	}
	var raw [][]byte
	for _, c := range v.certs {
		raw = append(raw, c.Raw)
	}
	if a := streams(oldCerts, raw); len(a) > 0 {
		dss.Set("Certs", a)
	}
	if a := streams(oldOCSPs, v.ocsps); len(a) > 0 {
		dss.Set("OCSPs", a)
	}
	if a := streams(oldCRLs, v.crls); len(a) > 0 {
		dss.Set("CRLs", a)
	}
	cat.Set("DSS", u.Add(dss))
	u.Set(u.Root(), cat)
	res, err := u.Bytes()
	if err != nil {
		return nil, err
	}
	return res.Data, nil
}

// chain nimmt eine Kette auf, ergänzt fehlende Aussteller über AIA bis zur
// Wurzel und holt für jedes Zertifikat außer der Wurzel OCSP und CRL.
func (v *validation) chain(certs []*x509.Certificate) {
	pool := append([]*x509.Certificate(nil), certs...)
	for i := 0; i < len(pool) && i < 16; i++ {
		c := pool[i]
		key := string(c.Raw)
		if v.seen[key] {
			continue
		}
		v.seen[key] = true
		v.certs = append(v.certs, c)
		if isSelfSigned(c) {
			continue
		}
		issuer := findIssuer(c, pool)
		if issuer == nil {
			for _, url := range c.IssuingCertificateURL {
				if b, err := v.get(url); err == nil {
					if ic, err := x509.ParseCertificate(b); err == nil && c.CheckSignatureFrom(ic) == nil {
						issuer = ic
						break
					}
				}
			}
		}
		if issuer == nil {
			v.errs = append(v.errs, fmt.Errorf("Aussteller von %q nicht gefunden (AIA fehlt oder ist nicht erreichbar)", c.Subject.CommonName))
			continue
		}
		pool = append(pool, issuer)
		v.revocation(c, issuer)
	}
}

func isSelfSigned(c *x509.Certificate) bool {
	return bytes.Equal(c.RawIssuer, c.RawSubject) && c.CheckSignatureFrom(c) == nil
}

func findIssuer(c *x509.Certificate, pool []*x509.Certificate) *x509.Certificate {
	for _, p := range pool {
		if bytes.Equal(c.RawIssuer, p.RawSubject) && c.CheckSignatureFrom(p) == nil {
			return p
		}
	}
	return nil
}

// revocation holt OCSP-Antwort und CRL zu einem Zertifikat. Ein gesperrtes
// Zertifikat ist ein Fehler, fehlt jede Sperrinformation ebenso.
func (v *validation) revocation(c, issuer *x509.Certificate) {
	got := false
	for _, url := range c.OCSPServer {
		req, err := ocsp.CreateRequest(c, issuer, &ocsp.RequestOptions{Hash: crypto.SHA256})
		if err != nil {
			continue
		}
		body, err := v.post(url, "application/ocsp-request", req)
		if err != nil {
			continue
		}
		r, err := ocsp.ParseResponseForCert(body, c, issuer)
		if err != nil {
			continue
		}
		if r.Status == ocsp.Revoked {
			v.errs = append(v.errs, fmt.Errorf("Zertifikat %q ist gesperrt seit %s", c.Subject.CommonName, r.RevokedAt.Format(time.DateOnly)))
			return
		}
		if r.Status == ocsp.Good {
			v.ocsps = append(v.ocsps, body)
			got = true
			break
		}
	}
	for _, url := range c.CRLDistributionPoints {
		b, err := v.get(url)
		if err != nil {
			continue
		}
		crl, err := x509.ParseRevocationList(b)
		if err != nil || crl.CheckSignatureFrom(issuer) != nil {
			continue
		}
		for _, e := range crl.RevokedCertificateEntries {
			if e.SerialNumber.Cmp(c.SerialNumber) == 0 {
				v.errs = append(v.errs, fmt.Errorf("Zertifikat %q ist laut CRL gesperrt", c.Subject.CommonName))
				return
			}
		}
		v.crls = append(v.crls, b)
		got = true
		break
	}
	if !got {
		v.errs = append(v.errs, fmt.Errorf("für Zertifikat %q ist weder OCSP noch CRL abrufbar", c.Subject.CommonName))
	}
}

const maxFetch = 4 << 20

func (v *validation) get(url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(v.ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return v.do(req)
}

func (v *validation) post(url, typ string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(v.ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", typ)
	return v.do(req)
}

func (v *validation) do(req *http.Request) ([]byte, error) {
	resp, err := v.opt.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s antwortet mit %s", req.URL, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxFetch))
}
