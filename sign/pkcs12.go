package sign

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"fmt"

	"software.sslmate.com/src/go-pkcs12"
)

// LoadPKCS12 reads the key and certificate chain from a PKCS#12 file.
// The key stays in memory. The password comes from the caller, the CLI
// reads it from an environment variable or prompts for it, never as an argument.
func LoadPKCS12(data []byte, password string) (*KeySigner, error) {
	key, cert, chain, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return nil, fmt.Errorf("PKCS#12-Datei nicht lesbar: %w", err)
	}
	s, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("Schlüssel in der PKCS#12-Datei kann nicht signieren (%T)", key)
	}
	// The root does not belong in the signature, intermediate certificates do.
	all := append([]*x509.Certificate{cert}, nonRoot(chain)...)
	return NewKeySigner(s, all...)
}

// nonRoot removes self-signed certificates (roots).
func nonRoot(certs []*x509.Certificate) []*x509.Certificate {
	var out []*x509.Certificate
	for _, c := range certs {
		if bytes.Equal(c.RawIssuer, c.RawSubject) && c.CheckSignatureFrom(c) == nil {
			continue
		}
		out = append(out, c)
	}
	return out
}
