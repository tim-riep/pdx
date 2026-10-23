// Package testpki builds a complete in-memory PKI for tests: root,
// intermediate CA, signing certificates (valid, revoked, expired), a
// certificate for the timestamp service and an HTTP server with OCSP, CRLs,
// issuer certificates and a timestamp service according to RFC 3161. None
// of it is stored, there are no real keys in the repo.
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
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"

	"github.com/tim-riep/pdx/sign"
)

// Identity is a key with a certificate.
type Identity struct {
	Key  crypto.Signer
	Cert *x509.Certificate
}

// PKI is a test PKI with a running server.
type PKI struct {
	Root, Inter              Identity
	Signer, Revoked, Expired Identity
	TSA                      Identity
	Server                   *httptest.Server

	base   string
	mu     sync.Mutex
	serial int64
}

// TSAURL is the address of the timestamp service.
func (p *PKI) TSAURL() string { return p.base + "/tsa" }

// Client is an HTTP client that routes every address to the server of the
// PKI, including those with the host name from NewForHost.
func (p *PKI) Client() *http.Client {
	addr := p.Server.Listener.Addr().String()
	return &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
	}}
}

// Chain returns the chain of an end-entity certificate without the root.
func (p *PKI) Chain(id Identity) []*x509.Certificate {
	return []*x509.Certificate{id.Cert, p.Inter.Cert}
}

// KeySigner returns a signer for an end-entity certificate.
func (p *PKI) KeySigner(t testing.TB, id Identity) *sign.KeySigner {
	t.Helper()
	s, err := sign.NewKeySigner(id.Key, p.Chain(id)...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// New builds the PKI and starts the server, which ends with the test.
func New(t testing.TB) *PKI {
	t.Helper()
	return NewForHost(t, "")
}

// NewForHost builds the PKI like New, but the addresses in the certificates
// use the host name host instead of 127.0.0.1, so that a container, for
// example, reaches the server. Client resolves host to the server in the test.
func NewForHost(t testing.TB, host string) *PKI {
	t.Helper()
	p := &PKI{}
	mux := http.NewServeMux()
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Server.Close)
	base := p.Server.URL
	if host != "" {
		_, port, _ := net.SplitHostPort(p.Server.Listener.Addr().String())
		base = "http://" + net.JoinHostPort(host, port)
	}
	p.base = base
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
	// The timestamp service needs the critical extension
	// timeStamping (RFC 3161 2.3), crypto/x509 does not mark it critical.
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

// issue creates a key and certificate, self-signed without an issuer.
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

// crl returns a CRL of the issuer with the revoked certificates.
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

// ocsp answers requests about certificates of the intermediate CA, signed by
// the intermediate CA itself.
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

// tsa is a timestamp service according to RFC 3161.
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
