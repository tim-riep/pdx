package pdx

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"strings"
	"testing"
	"time"

	"github.com/tim-riep/pdx/dssval"
	"github.com/tim-riep/pdx/internal/testpki"
	"github.com/tim-riep/pdx/internal/testutil"
	"github.com/tim-riep/pdx/pdf/reader"
	"github.com/tim-riep/pdx/sign"
)

type testSignerInfo struct {
	Version     int
	SID         asn1.RawValue
	DigestAlg   asn1.RawValue
	SignedAttrs asn1.RawValue `asn1:"tag:0"`
	SigAlg      asn1.RawValue
	Signature   []byte
	Unsigned    asn1.RawValue `asn1:"optional,tag:1"`
}

// sigDicts liefert alle Signaturdictionaries über die Felder.
func sigDicts(t *testing.T, f *reader.File) map[string]reader.Dict {
	t.Helper()
	out := map[string]reader.Dict{}
	var walk func(o reader.Object, prefix string)
	walk = func(o reader.Object, prefix string) {
		d := f.Dict(o)
		name := prefix
		if s, ok := f.Resolve(d["T"]).(reader.String); ok {
			if name != "" {
				name += "."
			}
			name += reader.TextString(s)
		}
		if v := f.Dict(d["V"]); v != nil && d["FT"] == reader.Name("Sig") {
			out[name] = v
		}
		for _, k := range f.Array(d["Kids"]) {
			walk(k, name)
		}
	}
	for _, o := range f.Array(f.Dict(f.Root()["AcroForm"])["Fields"]) {
		walk(o, "")
	}
	return out
}

// checkSig prüft eine Signatur unabhängig vom Erzeuger: ByteRange deckt die
// ganze Datei bis zu ihrem Ende außer Contents (ISO 32000-1 12.8.1), der
// Inhalt ist ein CMS mit gültiger Signatur über die Bytes. Geliefert wird
// das Ende der signierten Revision.
func checkSig(t *testing.T, data []byte, d reader.Dict, cert *x509.Certificate) (int, testSignerInfo, *sign.SignedDataInfo) {
	t.Helper()
	br := d["ByteRange"].(reader.Array)
	var n [4]int
	for i := range n {
		n[i] = int(br[i].(int64))
	}
	end := n[2] + n[3]
	if n[0] != 0 || n[2] > len(data) || end > len(data) || data[n[1]] != '<' || data[n[2]-1] != '>' {
		t.Fatalf("ByteRange %v passt nicht zur Datei", n)
	}
	if !bytes.HasPrefix(data[end-6:], []byte("%%EOF\n")) && !bytes.HasPrefix(data[end-5:], []byte("%%EOF")) {
		t.Errorf("ByteRange endet nicht an %%%%EOF")
	}
	content := append(append([]byte(nil), data[:n[1]]...), data[n[2]:end]...)
	der := []byte(d["Contents"].(reader.String))
	sd, err := sign.ParseSignedData(der)
	if err != nil {
		t.Fatal(err)
	}
	if len(sd.SignerInfos) != 1 {
		t.Fatalf("%d SignerInfos, verlangt genau eines (ISO 19005-2 6.4.3)", len(sd.SignerInfos))
	}
	var si testSignerInfo
	if _, err := asn1.Unmarshal(sd.SignerInfos[0], &si); err != nil {
		t.Fatal(err)
	}
	if cert != nil {
		if !sd.Certificates[0].Equal(cert) {
			t.Error("Signaturzertifikat fehlt im CMS")
		}
		attrs := append([]byte(nil), si.SignedAttrs.FullBytes...)
		attrs[0] = 0x31
		sum := sha256.Sum256(attrs)
		if !ecdsa.VerifyASN1(cert.PublicKey.(*ecdsa.PublicKey), sum[:], si.Signature) {
			t.Error("Signatur ungültig")
		}
		h := sha256.Sum256(content)
		if !bytes.Contains(si.SignedAttrs.Bytes, h[:]) {
			t.Error("message-digest passt nicht zur ByteRange")
		}
	}
	return end, si, sd
}

var oidTSToken = []byte{0x06, 0x0b, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x10, 0x02, 0x0e}

// TestSignPDF signiert die Musterrechnung in jeder Stufe mit unsichtbarem
// Feld. Die alten Bytes bleiben, das Signaturdictionary folgt ETSI EN
// 319 142-1 6.3, ab B-LT steht ein DSS im Katalog, B-LTA endet mit einem
// Dokument-Zeitstempel. veraPDF besteht danach weiter.
func TestSignPDF(t *testing.T) {
	pki := testpki.New(t)
	p, data := loadMuster(t)
	for _, prof := range []string{"pdfa-3b", "pdfa-4f"} {
		res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: prof}})
		if err != nil {
			t.Fatal(err)
		}
		for _, lvl := range []sign.Level{sign.LevelB, sign.LevelT, sign.LevelLT, sign.LevelLTA} {
			t.Run(prof+"/"+string(lvl), func(t *testing.T) {
				out, err := sign.SignPDF(context.Background(), res.PDF, pki.KeySigner(t, pki.Signer), sign.PDFOptions{
					Level: lvl, TSA: &sign.TSA{URL: pki.TSAURL()}, Reason: "Freigabe", Location: "Köln",
				})
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.HasPrefix(out, res.PDF) || !bytes.HasSuffix(out, []byte("%%EOF\n")) {
					t.Fatal("Ausgangsdatei verändert oder Ende falsch")
				}
				f, err := reader.Open(out)
				if err != nil {
					t.Fatal(err)
				}
				sigs := sigDicts(t, f)
				d := sigs["Signatur1"]
				if d == nil {
					t.Fatalf("Signatur1 fehlt, gefunden %v", sigs)
				}
				for k, want := range map[reader.Name]reader.Object{"Type": reader.Name("Sig"), "Filter": reader.Name("Adobe.PPKLite"), "SubFilter": reader.Name("ETSI.CAdES.detached")} {
					if d[k] != want {
						t.Errorf("%s = %v, verlangt %v", k, d[k], want)
					}
				}
				if d["M"] == nil || d["Cert"] != nil {
					t.Error("M fehlt oder Cert vorhanden (ETSI EN 319 142-1 6.3)")
				}
				end, si, _ := checkSig(t, out, d, pki.Signer.Cert)
				hasTS := bytes.Contains(si.Unsigned.Bytes, oidTSToken)
				if hasTS != (lvl != sign.LevelB) {
					t.Errorf("Zeitstempel im SignerInfo: %v", hasTS)
				}
				dss := f.Dict(f.Root()["DSS"])
				if (dss != nil) != (lvl == sign.LevelLT || lvl == sign.LevelLTA) {
					t.Fatalf("DSS vorhanden: %v", dss != nil)
				}
				if dss != nil {
					// Signatur, Zwischen-CA, Wurzel, TSA. OCSP für Signatur,
					// TSA und Zwischen-CA.
					if c := len(f.Array(dss["Certs"])); c < 4 {
						t.Errorf("%d Zertifikate im DSS", c)
					}
					if c := len(f.Array(dss["OCSPs"])); c < 3 {
						t.Errorf("%d OCSP-Antworten im DSS", c)
					}
					if dss["VRI"] != nil {
						t.Error("VRI sollte nicht verwendet werden")
					}
				}
				ts := sigs["Zeitstempel1"]
				if (ts != nil) != (lvl == sign.LevelLTA) {
					t.Fatalf("Dokument-Zeitstempel vorhanden: %v", ts != nil)
				}
				if ts != nil {
					if ts["Type"] != reader.Name("DocTimeStamp") || ts["SubFilter"] != reader.Name("ETSI.RFC3161") || ts["M"] != nil {
						t.Errorf("Dokument-Zeitstempel falsch: %v", ts)
					}
					tsEnd, _, tsd := checkSig(t, out, ts, nil)
					if tsEnd <= end {
						t.Error("Dokument-Zeitstempel deckt die Signatur nicht ab")
					}
					info, err := sign.ParseTimeStamp([]byte(ts["Contents"].(reader.String)))
					if err != nil || tsd.ContentType.String() != "1.2.840.113549.1.9.16.1.4" {
						t.Fatal("Inhalt ist kein Zeitstempel", err)
					}
					br := ts["ByteRange"].(reader.Array)
					a, b, c := int(br[1].(int64)), int(br[2].(int64)), int(br[3].(int64))
					h := sha256.Sum256(append(append([]byte(nil), out[:a]...), out[b:b+c]...))
					if !bytes.Equal(info.Imprint, h[:]) {
						t.Error("Abdruck des Dokument-Zeitstempels passt nicht")
					}
				}
				testutil.Conform(t, "signiert-"+prof+"-"+string(lvl)+".pdf", out, prof)
			})
		}
	}
}

// Das Signaturfeld aus der Vorlage wird signiert und zeigt danach
// Unterzeichner, Datum und Grund, mit demselben Text in Contents. Das
// Formular bleibt unter 3a mit UA-1 und unter 4 mit UA-2 konform. Ein
// zweites Signieren desselben Felds scheitert.
func TestSignFormField(t *testing.T) {
	pki := testpki.New(t)
	tpl, data := loadForm(t)
	for _, prof := range []string{"pdfa-3a,pdfua-1", "pdfa-4,pdfua-2"} {
		t.Run(prof, func(t *testing.T) {
			res, err := tpl.Render(data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: prof}})
			if err != nil {
				t.Fatal(err)
			}
			f0, _ := reader.Open(res.PDF)
			var name string
			for _, fl := range f0.Fields() {
				if fl.Type == "Sig" {
					name = fl.Name
				}
			}
			if name == "" {
				t.Fatal("Formular ohne Signaturfeld")
			}
			now := time.Now().UTC().Truncate(time.Minute)
			out, err := sign.SignPDF(context.Background(), res.PDF, pki.KeySigner(t, pki.Signer), sign.PDFOptions{
				Level: sign.LevelT, TSA: &sign.TSA{URL: pki.TSAURL()}, Field: name, Reason: "Antrag geprüft",
				Time: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			f, _ := reader.Open(out)
			d := sigDicts(t, f)[name]
			if d == nil {
				t.Fatal("Feld nicht signiert")
			}
			checkSig(t, out, d, pki.Signer.Cert)
			if len(f.Fields()) != len(f0.Fields()) {
				t.Error("neues Feld angelegt statt das vorhandene zu nutzen")
			}
			var widget reader.Dict
			for _, o := range f.Array(f.Dict(f.Root()["AcroForm"])["Fields"]) {
				if w := f.Dict(o); w["FT"] == reader.Name("Sig") {
					widget = w
				}
			}
			want := "Digital signiert von Erika Musterfrau, Datum: " + now.Format("2006-01-02 15:04") + " UTC, Grund: Antrag geprüft"
			if c, _ := f.Resolve(widget["Contents"]).(reader.String); reader.TextString(c) != want {
				t.Errorf("Contents %q", reader.TextString(c))
			}
			ap, ok := f.Resolve(f.Dict(widget["AP"])["N"]).(*reader.Stream)
			if !ok {
				t.Fatal("Erscheinungsbild fehlt")
			}
			raw, err := f.Decode(ap)
			if err != nil || !bytes.Contains(raw, []byte("/Bg Do")) || !bytes.Contains(raw, []byte("Tj")) {
				t.Errorf("Erscheinungsbild ohne Hintergrund oder Text: %s", raw)
			}
			testutil.Conform(t, "formular-signiert.pdf", out, prof)
			if _, err := sign.SignPDF(context.Background(), out, pki.KeySigner(t, pki.Signer), sign.PDFOptions{Field: name}); err == nil || !strings.Contains(err.Error(), "bereits signiert") {
				t.Errorf("zweites Signieren: %v", err)
			}
			if _, err := sign.SignPDF(context.Background(), out, pki.KeySigner(t, pki.Signer), sign.PDFOptions{Field: "gibtsnicht"}); err == nil {
				t.Error("fehlendes Feld nicht gemeldet")
			}
		})
	}
}

// Abgelaufene Zertifikate scheitern sofort, gesperrte spätestens beim
// Einsammeln der Sperrinformation für B-LT. Ohne Zeitstempeldienst gibt es
// kein B-T.
func TestSignPDFRejects(t *testing.T) {
	pki := testpki.New(t)
	p, data := loadMuster(t)
	res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-3b"}})
	if err != nil {
		t.Fatal(err)
	}
	tsa := &sign.TSA{URL: pki.TSAURL()}
	cases := []struct {
		name string
		id   testpki.Identity
		opt  sign.PDFOptions
		want string
	}{
		{"abgelaufen", pki.Expired, sign.PDFOptions{}, "abgelaufen"},
		{"gesperrt", pki.Revoked, sign.PDFOptions{Level: sign.LevelLT, TSA: tsa}, "gesperrt"},
		{"ohne TSA", pki.Signer, sign.PDFOptions{Level: sign.LevelT}, "Zeitstempeldienst"},
		{"Stufe", pki.Signer, sign.PDFOptions{Level: "B-X"}, "unbekannt"},
		{"zu klein", pki.Signer, sign.PDFOptions{Reserve: 64}, "reserviert"},
		{"Zukunft", pki.Signer, sign.PDFOptions{Time: time.Now().Add(-72 * time.Hour)}, "gilt erst"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := sign.SignPDF(context.Background(), res.PDF, pki.KeySigner(t, c.id), c.opt)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Fehler %v, erwartet %q", err, c.want)
			}
		})
	}
}

// TestSignDSS prüft jede Stufe mit dem EU DSS Validator (PDX_DSS=1, Image
// aus dssval/build.sh). Vertrauensanker ist nur die Wurzel der Test-PKI.
// Verlangt ist TOTAL_PASSED mit dem Format der Stufe, danach besteht auch
// veraPDF.
func TestSignDSS(t *testing.T) {
	if !dssval.Enabled() {
		t.Skip("DSS-Prüfung nur mit " + dssval.EnvEnable + "=1")
	}
	pki := testpki.NewForHost(t, dssval.HostAlias)
	ctx := context.Background()
	srv, err := dssval.Start(ctx, []*x509.Certificate{pki.Root.Cert})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Stop)
	p, data := loadMuster(t)
	for _, prof := range []string{"pdfa-3b", "pdfa-4f"} {
		res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: prof}})
		if err != nil {
			t.Fatal(err)
		}
		for _, lvl := range []sign.Level{sign.LevelB, sign.LevelT, sign.LevelLT, sign.LevelLTA} {
			t.Run(prof+"/"+string(lvl), func(t *testing.T) {
				out, err := sign.SignPDF(ctx, res.PDF, pki.KeySigner(t, pki.Signer), sign.PDFOptions{
					Level: lvl, TSA: &sign.TSA{URL: pki.TSAURL(), Client: pki.Client()}, Client: pki.Client(),
				})
				if err != nil {
					t.Fatal(err)
				}
				sigs, err := srv.Validate(ctx, out)
				if err != nil {
					t.Fatal(err)
				}
				for _, s := range sigs {
					t.Logf("%s %s %s %s %s", s.Type, s.Format, s.Indication, s.SubIndication, strings.Join(s.Errors, "; "))
					switch s.Type {
					case "Signature":
						if s.Indication != "TOTAL_PASSED" || s.Format != "PAdES-BASELINE-"+strings.TrimPrefix(string(lvl), "B-") {
							t.Errorf("Signatur: %s %s %s, Fehler %v", s.Format, s.Indication, s.SubIndication, s.Errors)
						}
					case "Timestamp":
						if s.Indication != "PASSED" && s.Indication != "TOTAL_PASSED" {
							t.Errorf("Zeitstempel: %s %s, Fehler %v", s.Indication, s.SubIndication, s.Errors)
						}
					}
				}
				testutil.Conform(t, "dss-"+prof+"-"+string(lvl)+".pdf", out, prof)
			})
		}
	}
}

// Ein unsichtbares Signaturfeld (Rect 0, ohne Fläche und ohne Inhalt für
// Hilfstechnik) lässt Formulare unter UA-1 und UA-2 konform.
func TestSignInvisibleUA(t *testing.T) {
	pki := testpki.New(t)
	tpl, data := loadForm(t)
	for _, prof := range []string{"pdfa-3a,pdfua-1", "pdfa-4,pdfua-2"} {
		t.Run(prof, func(t *testing.T) {
			res, err := tpl.Render(data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: prof}})
			if err != nil {
				t.Fatal(err)
			}
			out, err := sign.SignPDF(context.Background(), res.PDF, pki.KeySigner(t, pki.Signer), sign.PDFOptions{})
			if err != nil {
				t.Fatal(err)
			}
			f, _ := reader.Open(out)
			checkSig(t, out, sigDicts(t, f)["Signatur1"], pki.Signer.Cert)
			testutil.Conform(t, "unsichtbar.pdf", out, prof)
		})
	}
}

// FuzzSignPDF signiert beliebige Bytes in B-B. Erlaubt ist ein Fehler,
// verboten sind Absturz und eine Ausgabe, die die Eingabe verändert.
func FuzzSignPDF(f *testing.F) {
	p, data := loadMuster(f)
	res, err := RenderTemplate(p, data, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-3b"}})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(res.PDF)
	tpl, fdata := loadForm(f)
	if r, err := tpl.Render(fdata, TemplateOptions{Options: Options{CreationDate: templateDate, Profile: "pdfa-2b"}}); err == nil {
		f.Add(r.PDF)
	}
	f.Add([]byte("%PDF-1.7\n1 0 obj<</Type/Catalog>>endobj\ntrailer<</Root 1 0 R>>\n%%EOF"))
	pki := testpki.New(f)
	s := pki.KeySigner(f, pki.Signer)
	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := sign.SignPDF(context.Background(), in, s, sign.PDFOptions{})
		if err == nil && !bytes.HasPrefix(out, in) {
			t.Fatal("Eingabe verändert")
		}
	})
}
