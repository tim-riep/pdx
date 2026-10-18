// Package testpki baut für Tests eine vollständige PKI im Speicher: Wurzel,
// Zwischen-CA, Signaturzertifikate (gültig, gesperrt, abgelaufen), ein
// Zertifikat für den Zeitstempeldienst und einen HTTP-Server mit OCSP, CRLs,
// Ausstellerzertifikaten und einem Zeitstempeldienst nach RFC 3161. Nichts
// davon wird gespeichert, echte Schlüssel gibt es im Repo nicht.
package testpki

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"

	"github.com/tim-riep/pdx/sign"
)

// Identity ist ein Schlüssel mit Zertifikat.
type Identity struct {
	Key  crypto.Signer
	Cert *x509.Certificate
}

// PKI ist eine Test-PKI mit laufendem Server.
type PKI struct {
	Root, Inter              Identity
	Signer, Revoked, Expired Identity
	TSA                      Identity
	Server                   *httptest.Server

	mu     sync.Mutex
	serial int64
}

// TSAURL ist die Adresse des Zeitstempeldienstes.
func (p *PKI) TSAURL() string { return p.Server.URL + "/tsa" }

// Chain liefert die Kette eines Endzertifikats ohne Wurzel.
func (p *PKI) Chain(id Identity) []*x509.Certificate {
	return []*x509.Certificate{id.Cert, p.Inter.Cert}
}

// KeySigner liefert einen Signer für ein Endzertifikat.
func (p *PKI) KeySigner(t testing.TB, id Identity) *sign.KeySigner {
	t.Helper()
	s, err := sign.NewKeySigner(id.Key, p.Chain(id)...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// New baut die PKI und startet den Server, der mit dem Test endet.
func New(t testing.TB) *PKI {
	t.Helper()
	p := &PKI{}
	mux := http.NewServeMux()
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Server.Close)
	base := p.Server.URL
	now := time.Now().Add(-time.Minute)

	p.Root = p.issue(t, nil, &x509.Certificate{
		Subject:   pkix.Name{CommonName: "pdx Test Wurzel", Organization: []string{"pdx Test"}, Country: []string{"DE"}},
		NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(10 * 365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	})
	p.Inter = p.issue(t, &p.Root, &x509.Certificate{
		Subject:   pkix.Name{CommonName: "pdx Test Zwischen-CA", Organization: []string{"pdx Test"}, Country: []string{"DE"}},
		NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(5 * 365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, MaxPathLen: 0, MaxPathLenZero: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		OCSPServer:            []string{base + "/ocsp"},
		IssuingCertificateURL: []string{base + "/root.crt"},
		CRLDistributionPoints: []string{base + "/root.crl"},
	})
	leaf := func(cn string, from, to time.Time, eku []x509.ExtKeyUsage, extra []pkix.Extension) Identity {
		return p.issue(t, &p.Inter, &x509.Certificate{
			Subject:   pkix.Name{CommonName: cn, Organization: []string{"pdx Test"}, Country: []string{"DE"}},
			NotBefore: from, NotAfter: to,
			KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageContentCommitment,
			ExtKeyUsage:           eku,
			ExtraExtensions:       extra,
			OCSPServer:            []string{base + "/ocsp"},
			IssuingCertificateURL: []string{base + "/inter.crt"},
			CRLDistributionPoints: []string{base + "/inter.crl"},
		})
	}
	p.Signer = leaf("Erika Musterfrau", now.Add(-time.Hour), now.Add(365*24*time.Hour), nil, nil)
	p.Revoked = leaf("Gesperrt", now.Add(-time.Hour), now.Add(365*24*time.Hour), nil, nil)
	p.Expired = leaf("Abgelaufen", now.Add(-48*time.Hour), now.Add(-24*time.Hour), nil, nil)
	// Der Zeitstempeldienst braucht die kritische Erweiterung
	// timeStamping (RFC 3161 2.3), crypto/x509 setzt sie nicht kritisch.
	ekuTS, _ := asn1.Marshal([]asn1.ObjectIdentifier{{1, 3, 6, 1, 5, 5, 7, 3, 8}})
	p.TSA = leaf("pdx Test Zeitstempel", now.Add(-time.Hour), now.Add(365*24*time.Hour), nil,
		[]pkix.Extension{{Id: asn1.ObjectIdentifier{2, 5, 29, 37}, Critical: true, Value: ekuTS}})

	der := func(c *x509.Certificate) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/pkix-cert")
			w.Write(c.Raw)
		}
	}
	mux.Handle("/root.crt", der(p.Root.Cert))
	mux.Handle("/inter.crt", der(p.Inter.Cert))
	mux.HandleFunc("/root.crl", p.crl(t, p.Root, nil))
	mux.HandleFunc("/inter.crl", p.crl(t, p.Inter, p.Revoked.Cert))
	mux.HandleFunc("/ocsp", p.ocsp)
	mux.HandleFunc("/tsa", p.tsa)
	return p
}

func (p *PKI) nextSerial() *big.Int {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.serial++
	return big.NewInt(1000 + p.serial)
}

// issue erzeugt Schlüssel und Zertifikat, selbst signiert ohne Aussteller.
func (p *PKI) issue(t testing.TB, issuer *Identity, tpl *x509.Certificate) Identity {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl.SerialNumber = p.nextSerial()
	parent, signer := tpl, crypto.Signer(key)
	if issuer != nil {
		parent, signer = issuer.Cert, issuer.Key
	}
	raw, err := x509.CreateCertificate(rand.Reader, tpl, parent, key.Public(), signer)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		t.Fatal(err)
	}
	return Identity{Key: key, Cert: cert}
}

// crl liefert eine CRL des Ausstellers mit den gesperrten Zertifikaten.
func (p *PKI) crl(t testing.TB, issuer Identity, revoked *x509.Certificate) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tpl := &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: time.Now().Add(-time.Minute), NextUpdate: time.Now().Add(24 * time.Hour)}
		if revoked != nil {
			tpl.RevokedCertificateEntries = []x509.RevocationListEntry{{SerialNumber: revoked.SerialNumber, RevocationTime: time.Now().Add(-30 * time.Minute)}}
		}
		der, err := x509.CreateRevocationList(rand.Reader, tpl, issuer.Cert, issuer.Key)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/pkix-crl")
		w.Write(der)
	}
}

// ocsp beantwortet Anfragen zu Zertifikaten der Zwischen-CA, signiert von
// der Zwischen-CA selbst.
func (p *PKI) ocsp(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	req, err := ocsp.ParseRequest(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	issuer := p.Inter
	if req.SerialNumber.Cmp(p.Inter.Cert.SerialNumber) == 0 {
		issuer = p.Root
	}
	tpl := ocsp.Response{Status: ocsp.Good, SerialNumber: req.SerialNumber,
		ThisUpdate: time.Now().Add(-time.Minute), NextUpdate: time.Now().Add(24 * time.Hour)}
	if req.SerialNumber.Cmp(p.Revoked.Cert.SerialNumber) == 0 {
		tpl.Status = ocsp.Revoked
		tpl.RevokedAt = time.Now().Add(-30 * time.Minute)
		tpl.RevocationReason = ocsp.KeyCompromise
	}
	der, err := ocsp.CreateResponse(issuer.Cert, issuer.Cert, tpl, issuer.Key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/ocsp-response")
	w.Write(der)
}

type timeStampReq struct {
	Version int
	Imprint struct {
		Alg struct {
			Algorithm  asn1.ObjectIdentifier
			Parameters asn1.RawValue `asn1:"optional"`
		}
		Hash []byte
	}
	Policy  asn1.ObjectIdentifier `asn1:"optional"`
	Nonce   *big.Int              `asn1:"optional"`
	CertReq bool                  `asn1:"optional,default:false"`
}

// tsa ist ein Zeitstempeldienst nach RFC 3161.
func (p *PKI) tsa(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	var req timeStampReq
	if _, err := asn1.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	imprint, _ := asn1.Marshal(struct {
		Alg struct {
			Algorithm asn1.ObjectIdentifier
		}
		Hash []byte
	}{struct{ Algorithm asn1.ObjectIdentifier }{req.Imprint.Alg.Algorithm}, req.Imprint.Hash})
	info := struct {
		Version int
		Policy  asn1.ObjectIdentifier
		Imprint asn1.RawValue
		Serial  *big.Int
		GenTime time.Time `asn1:"generalized"`
		Nonce   *big.Int  `asn1:"optional"`
	}{1, asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 99999, 1}, asn1.RawValue{FullBytes: imprint},
		p.nextSerial(), time.Now().UTC().Truncate(time.Second), req.Nonce}
	tst, err := asn1.Marshal(info)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s, err := sign.NewKeySigner(p.TSA.Key, p.TSA.Cert, p.Inter.Cert)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	token, err := sign.SignedData(context.Background(), s, tst, sign.SignedOptions{ContentType: sign.OIDTSTInfo})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status, _ := asn1.Marshal(struct{ Status int }{0})
	resp, _ := asn1.Marshal(struct {
		Status asn1.RawValue
		Token  asn1.RawValue
	}{asn1.RawValue{FullBytes: status}, asn1.RawValue{FullBytes: token}})
	w.Header().Set("Content-Type", "application/timestamp-reply")
	w.Write(resp)
}
