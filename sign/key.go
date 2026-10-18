package sign

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"errors"
)

// KeySigner signiert mit einem Schlüssel im Speicher, etwa aus einer
// PKCS#12-Datei. Der Schlüssel wird nie geschrieben oder protokolliert.
type KeySigner struct {
	key   crypto.Signer
	chain []*x509.Certificate
}

// NewKeySigner legt einen Signer an. chain beginnt mit dem
// Signaturzertifikat, dessen öffentlicher Schlüssel zu key passen muss.
func NewKeySigner(key crypto.Signer, chain ...*x509.Certificate) (*KeySigner, error) {
	if len(chain) == 0 {
		return nil, errors.New("Signaturzertifikat fehlt")
	}
	if !samePublic(key.Public(), chain[0].PublicKey) {
		return nil, errors.New("der Schlüssel passt nicht zum Signaturzertifikat")
	}
	return &KeySigner{key: key, chain: chain}, nil
}

func samePublic(a, b crypto.PublicKey) bool {
	type equal interface{ Equal(crypto.PublicKey) bool }
	e, ok := a.(equal)
	return ok && e.Equal(b)
}

// Certificates liefert das Signaturzertifikat und seine Kette.
func (k *KeySigner) Certificates() []*x509.Certificate { return k.chain }

// SignDigest signiert einen Hashwert.
func (k *KeySigner) SignDigest(_ context.Context, hash crypto.Hash, digest []byte) ([]byte, error) {
	return k.key.Sign(rand.Reader, digest, hash)
}

// String verrät nichts über den Schlüssel, falls ein Signer versehentlich
// ausgegeben wird.
func (k *KeySigner) String() string { return "KeySigner(" + k.chain[0].Subject.CommonName + ")" }
