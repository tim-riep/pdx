package sign

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"errors"
)

// KeySigner signs with a key in memory, for example from a
// PKCS#12 file. The key is never written or logged.
type KeySigner struct {
	key   crypto.Signer
	chain []*x509.Certificate
}

// NewKeySigner creates a Signer. chain starts with the
// signing certificate, whose public key must match key.
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

// Certificates returns the signing certificate and its chain.
func (k *KeySigner) Certificates() []*x509.Certificate { return k.chain }

// SignDigest signs a hash value.
func (k *KeySigner) SignDigest(_ context.Context, hash crypto.Hash, digest []byte) ([]byte, error) {
	return k.key.Sign(rand.Reader, digest, hash)
}

// String reveals nothing about the key in case a Signer is accidentally
// printed.
func (k *KeySigner) String() string { return "KeySigner(" + k.chain[0].Subject.CommonName + ")" }
