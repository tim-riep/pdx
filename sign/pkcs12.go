package sign

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"fmt"

	"software.sslmate.com/src/go-pkcs12"
)

// LoadPKCS12 liest Schlüssel und Zertifikatskette aus einer PKCS#12-Datei.
// Der Schlüssel bleibt im Speicher. Das Passwort kommt vom Aufrufer, die CLI
// liest es aus einer Umgebungsvariable oder fragt es ab, nie als Argument.
func LoadPKCS12(data []byte, password string) (*KeySigner, error) {
	key, cert, chain, err := pkcs12.DecodeChain(data, password)
	if err != nil {
		return nil, fmt.Errorf("PKCS#12-Datei nicht lesbar: %w", err)
	}
	s, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("Schlüssel in der PKCS#12-Datei kann nicht signieren (%T)", key)
	}
	// Die Wurzel gehört nicht in die Signatur, Zwischenzertifikate schon.
	all := append([]*x509.Certificate{cert}, nonRoot(chain)...)
	return NewKeySigner(s, all...)
}

// nonRoot entfernt selbst signierte Zertifikate (Wurzeln).
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
