package sign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// RemoteSigner signs via a signing service, the key stays with the
// service (for example in front of an HSM). The protocol is deliberately
// small, JSON over HTTPS with a bearer token:
//
//	GET  {URL}/certificates  → {"certificates": [DER base64, ...]}
//	                            signing certificate first, without root
//	POST {URL}/sign          ← {"hash": "SHA-256", "digest": base64}
//	                         → {"signature": base64}
//
// RSA returns PKCS#1 v1.5, ECDSA the ASN.1 signature. The token comes from
// the caller, the CLI reads it from an environment variable. It appears in
// no error message.
type RemoteSigner struct {
	URL    string
	Token  string
	Client *http.Client
	chain  []*x509.Certificate
}

// maxRemoteResponse limits responses of the signing service.
const maxRemoteResponse = 1 << 20

var hashNames = map[crypto.Hash]string{crypto.SHA256: "SHA-256", crypto.SHA384: "SHA-384", crypto.SHA512: "SHA-512"}

// NewRemoteSigner fetches the certificates of the service. Unencrypted
// HTTP is only allowed for localhost.
func NewRemoteSigner(ctx context.Context, url, token string, client *http.Client) (*RemoteSigner, error) {
	url = strings.TrimSuffix(url, "/")
	if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://127.0.0.1") && !strings.HasPrefix(url, "http://localhost") {
		return nil, fmt.Errorf("Signaturdienst %s: nur HTTPS ist erlaubt", url)
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	r := &RemoteSigner{URL: url, Token: token, Client: client}
	var resp struct {
		Certificates [][]byte `json:"certificates"`
	}
	if err := r.call(ctx, http.MethodGet, "/certificates", nil, &resp); err != nil {
		return nil, err
	}
	for _, der := range resp.Certificates {
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return nil, fmt.Errorf("Signaturdienst: Zertifikat nicht lesbar: %v", err)
		}
		r.chain = append(r.chain, c)
	}
	if len(r.chain) == 0 {
		return nil, errors.New("Signaturdienst liefert kein Zertifikat")
	}
	return r, nil
}

// Certificates returns the signing certificate and its chain.
func (r *RemoteSigner) Certificates() []*x509.Certificate { return r.chain }

// SignDigest has the service sign the hash value and checks the
// signature against the certificate before it goes into the PDF.
func (r *RemoteSigner) SignDigest(ctx context.Context, hash crypto.Hash, digest []byte) ([]byte, error) {
	name, ok := hashNames[hash]
	if !ok {
		return nil, fmt.Errorf("Hashverfahren %v wird nicht unterstützt", hash)
	}
	var resp struct {
		Signature []byte `json:"signature"`
	}
	if err := r.call(ctx, http.MethodPost, "/sign", map[string]any{"hash": name, "digest": digest}, &resp); err != nil {
		return nil, err
	}
	valid := false
	switch pub := r.chain[0].PublicKey.(type) {
	case *ecdsa.PublicKey:
		valid = ecdsa.VerifyASN1(pub, digest, resp.Signature)
	case *rsa.PublicKey:
		valid = rsa.VerifyPKCS1v15(pub, hash, digest, resp.Signature) == nil
	}
	if !valid {
		return nil, errors.New("Signaturdienst: Signatur passt nicht zum Zertifikat")
	}
	return resp.Signature, nil
}

func (r *RemoteSigner) call(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, r.URL+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if r.Token != "" {
		req.Header.Set("Authorization", "Bearer "+r.Token)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return fmt.Errorf("Signaturdienst: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteResponse))
	if err != nil {
		return fmt.Errorf("Signaturdienst: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Signaturdienst %s antwortet mit %s", path, resp.Status)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("Signaturdienst %s: Antwort nicht lesbar", path)
	}
	return nil
}

// String does not reveal the token.
func (r *RemoteSigner) String() string { return "RemoteSigner(" + r.URL + ")" }
