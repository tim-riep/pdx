package sign_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ocsp"
	"software.sslmate.com/src/go-pkcs12"

	"github.com/tim-riep/pdx/internal/testpki"
	"github.com/tim-riep/pdx/sign"
)

type signerInfo struct {
	Version     int
	SID         asn1.RawValue
	DigestAlg   asn1.RawValue
	SignedAttrs asn1.RawValue `asn1:"tag:0"`
	SigAlg      asn1.RawValue
	Signature   []byte
	Unsigned    asn1.RawValue `asn1:"optional,tag:1"`
}

type attribute struct {
	Type   asn1.ObjectIdentifier
	Values asn1.RawValue `asn1:"set"`
}

// verify checks the signature of a SignerInfo independently of the producer and
// returns the signed attributes by type.
func verify(t *testing.T, si []byte, cert *x509.Certificate, content []byte) (map[string][]byte, signerInfo) {
	t.Helper()
	var s signerInfo
	if _, err := asn1.Unmarshal(si, &s); err != nil {
		t.Fatal(err)
	}
	attrs := append([]byte(nil), s.SignedAttrs.FullBytes...)
	attrs[0] = 0x31 // the encoding as SET is signed
	sum := sha256.Sum256(attrs)
	if !ecdsa.VerifyASN1(cert.PublicKey.(*ecdsa.PublicKey), sum[:], s.Signature) {
		t.Fatal("Signatur ungültig")
	}
	out := map[string][]byte{}
	for rest := s.SignedAttrs.Bytes; len(rest) > 0; {
		var a attribute
		var err error
		if rest, err = asn1.Unmarshal(rest, &a); err != nil {
			t.Fatal(err)
		}
		out[a.Type.String()] = a.Values.Bytes
	}
	var md []byte
	asn1.Unmarshal(out["1.2.840.113549.1.9.4"], &md)
	if c := sha256.Sum256(content); !bytes.Equal(md, c[:]) {
		t.Error("message-digest passt nicht zum Inhalt")
	}
	return out, s
}

// SignedData carries the attributes required by PAdES, no signing-time,
// and can be verified with the certificate. With a timestamp as unsigned
// attribute, B-T results.
func TestSignedDataPAdES(t *testing.T) {
	pki := testpki.New(t)
	s := pki.KeySigner(t, pki.Signer)
	content := []byte("die Bytes der ByteRange")
	tsa := &sign.TSA{URL: pki.TSAURL()}
	der, err := sign.SignedData(context.Background(), s, content, sign.SignedOptions{Detached: true,
		Unsigned: func(sig []byte) ([][]byte, error) {
			tok, err := tsa.Timestamp(context.Background(), sig)
			if err != nil {
				return nil, err
			}
			return [][]byte{sign.TimeStampAttribute(tok)}, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	sd, err := sign.ParseSignedData(der)
	if err != nil {
		t.Fatal(err)
	}
	if len(sd.SignerInfos) != 1 || len(sd.Content) != 0 || len(sd.Certificates) != 2 || !sd.Certificates[0].Equal(pki.Signer.Cert) {
		t.Fatalf("Aufbau: %d SignerInfos, Inhalt %d Bytes, %d Zertifikate", len(sd.SignerInfos), len(sd.Content), len(sd.Certificates))
	}
	attrs, si := verify(t, sd.SignerInfos[0], pki.Signer.Cert, content)
	for _, o := range []string{"1.2.840.113549.1.9.3", "1.2.840.113549.1.9.4", "1.2.840.113549.1.9.16.2.47"} {
		if attrs[o] == nil {
			t.Errorf("signiertes Attribut %s fehlt", o)
		}
	}
	if attrs["1.2.840.113549.1.9.5"] != nil {
		t.Error("signing-time ist unter PAdES verboten")
	}
	if !bytes.Contains(si.Unsigned.Bytes, []byte{0x06, 0x0b, 0x2a, 0x86, 0x48, 0x86, 0xf7, 0x0d, 0x01, 0x09, 0x10, 0x02, 0x0e}) {
		t.Error("Zeitstempel als unsigniertes Attribut fehlt")
	}
}

// The timestamp service of the test PKI returns tokens with imprint, time and
// certificate, OCSP and CRL know the revoked certificate.
func TestTestPKI(t *testing.T) {
	pki := testpki.New(t)
	tok, err := (&sign.TSA{URL: pki.TSAURL()}).Timestamp(context.Background(), []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := sign.ParseTimeStamp(tok)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.Time) > time.Minute || len(info.Certificates) == 0 || !info.Certificates[0].Equal(pki.TSA.Cert) {
		t.Errorf("Zeitstempel %+v", info)
	}
	sd, _ := sign.ParseSignedData(tok)
	verify(t, sd.SignerInfos[0], pki.TSA.Cert, sd.Content)

	status := func(c *x509.Certificate) int {
		req, err := ocsp.CreateRequest(c, pki.Inter.Cert, &ocsp.RequestOptions{Hash: crypto.SHA256})
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.Post(c.OCSPServer[0], "application/ocsp-request", bytes.NewReader(req))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		r, err := ocsp.ParseResponseForCert(body, c, pki.Inter.Cert)
		if err != nil {
			t.Fatal(err)
		}
		return r.Status
	}
	if status(pki.Signer.Cert) != ocsp.Good || status(pki.Revoked.Cert) != ocsp.Revoked {
		t.Error("OCSP-Status falsch")
	}
	resp, err := http.Get(pki.Signer.Cert.CRLDistributionPoints[0])
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	crl, err := x509.ParseRevocationList(body)
	if err != nil || crl.CheckSignatureFrom(pki.Inter.Cert) != nil {
		t.Fatalf("CRL: %v", err)
	}
	if len(crl.RevokedCertificateEntries) != 1 || crl.RevokedCertificateEntries[0].SerialNumber.Cmp(pki.Revoked.Cert.SerialNumber) != 0 {
		t.Error("CRL nennt das gesperrte Zertifikat nicht")
	}
}

// LoadPKCS12 reads key and chain, the root is dropped, a wrong
// password fails. The file is created in the test.
func TestLoadPKCS12(t *testing.T) {
	pki := testpki.New(t)
	p12, err := pkcs12.Modern.Encode(pki.Signer.Key, pki.Signer.Cert, []*x509.Certificate{pki.Inter.Cert, pki.Root.Cert}, "geheim")
	if err != nil {
		t.Fatal(err)
	}
	s, err := sign.LoadPKCS12(p12, "geheim")
	if err != nil {
		t.Fatal(err)
	}
	certs := s.Certificates()
	if len(certs) != 2 || !certs[0].Equal(pki.Signer.Cert) || !certs[1].Equal(pki.Inter.Cert) {
		t.Errorf("Kette falsch: %d Zertifikate", len(certs))
	}
	if _, err := sign.LoadPKCS12(p12, "falsch"); err == nil {
		t.Error("falsches Passwort nicht erkannt")
	}
}

// remoteService is a signing service following the protocol of RemoteSigner.
func remoteService(t *testing.T, pki *testpki.PKI, token string, bad bool) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "nicht angemeldet", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/certificates":
			json.NewEncoder(w).Encode(map[string]any{"certificates": [][]byte{pki.Signer.Cert.Raw, pki.Inter.Cert.Raw}})
		case "/sign":
			var req struct {
				Hash   string
				Digest []byte
			}
			json.NewDecoder(r.Body).Decode(&req)
			key := pki.Signer.Key
			if bad {
				key = pki.Revoked.Key
			}
			sig, _ := key.Sign(rand.Reader, req.Digest, crypto.SHA256)
			json.NewEncoder(w).Encode(map[string]any{"signature": sig})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// RemoteSigner fetches chain and signature from the service, checks the signature and
// names the token in no message.
func TestRemoteSigner(t *testing.T) {
	pki := testpki.New(t)
	ctx := context.Background()
	srv := remoteService(t, pki, "tok-123", false)
	s, err := sign.NewRemoteSigner(ctx, srv.URL, "tok-123", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Certificates()) != 2 {
		t.Fatalf("%d Zertifikate", len(s.Certificates()))
	}
	content := []byte("Inhalt")
	der, err := sign.SignedData(ctx, s, content, sign.SignedOptions{Detached: true})
	if err != nil {
		t.Fatal(err)
	}
	sd, _ := sign.ParseSignedData(der)
	verify(t, sd.SignerInfos[0], pki.Signer.Cert, content)

	if _, err := sign.NewRemoteSigner(ctx, srv.URL, "falsch", nil); err == nil || strings.Contains(err.Error(), "falsch") {
		t.Errorf("falscher Token: %v", err)
	}
	if _, err := sign.NewRemoteSigner(ctx, "http://example.org", "", nil); err == nil {
		t.Error("HTTP ohne TLS erlaubt")
	}
	bad, _ := sign.NewRemoteSigner(ctx, remoteService(t, pki, "t", true).URL, "t", nil)
	if _, err := sign.SignedData(ctx, bad, content, sign.SignedOptions{}); err == nil {
		t.Error("Signatur mit falschem Schlüssel angenommen")
	}
}
