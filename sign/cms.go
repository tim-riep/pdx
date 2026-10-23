package sign

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
)

// Object identifiers (RFC 5652, RFC 5035, RFC 3161, RFC 5754, RFC 8017,
// RFC 5758).
var (
	oidData                 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 1}
	oidSignedData           = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 7, 2}
	oidContentType          = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 3}
	oidMessageDigest        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
	oidSigningCertificateV2 = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 47}
	oidTimeStampToken       = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 2, 14}
	oidTSTInfo              = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 16, 1, 4}
	oidSHA256               = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidSHA384               = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidSHA512               = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	oidSHA256WithRSA        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 11}
	oidSHA384WithRSA        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 12}
	oidSHA512WithRSA        = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 13}
	oidECDSAWithSHA256      = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 2}
	oidECDSAWithSHA384      = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 3}
	oidECDSAWithSHA512      = asn1.ObjectIdentifier{1, 2, 840, 10045, 4, 3, 4}
)

var hashOIDs = map[crypto.Hash]asn1.ObjectIdentifier{crypto.SHA256: oidSHA256, crypto.SHA384: oidSHA384, crypto.SHA512: oidSHA512}

// Signer signs a hash value. The key never leaves its
// storage location, whether a file in memory, an HSM or a signing service.
type Signer interface {
	// Certificates returns the signing certificate and its chain up to
	// but excluding the root.
	Certificates() []*x509.Certificate
	// SignDigest signs the hash value digest of the algorithm hash. RSA
	// returns PKCS#1 v1.5, ECDSA the ASN.1 signature.
	SignDigest(ctx context.Context, hash crypto.Hash, digest []byte) ([]byte, error)
}

// signatureAlgorithm returns the signature algorithm for the key.
func signatureAlgorithm(pub crypto.PublicKey, hash crypto.Hash) ([]byte, error) {
	switch pub.(type) {
	case *rsa.PublicKey:
		o := map[crypto.Hash]asn1.ObjectIdentifier{crypto.SHA256: oidSHA256WithRSA, crypto.SHA384: oidSHA384WithRSA, crypto.SHA512: oidSHA512WithRSA}[hash]
		if o == nil {
			return nil, fmt.Errorf("Hashverfahren %v wird für RSA nicht unterstützt", hash)
		}
		return algorithm(o, true), nil
	case *ecdsa.PublicKey:
		o := map[crypto.Hash]asn1.ObjectIdentifier{crypto.SHA256: oidECDSAWithSHA256, crypto.SHA384: oidECDSAWithSHA384, crypto.SHA512: oidECDSAWithSHA512}[hash]
		if o == nil {
			return nil, fmt.Errorf("Hashverfahren %v wird für ECDSA nicht unterstützt", hash)
		}
		return algorithm(o, false), nil
	}
	return nil, fmt.Errorf("Schlüsseltyp %T wird nicht unterstützt, erwartet RSA oder ECDSA", pub)
}

// attribute encodes an attribute with one value.
func attribute(o asn1.ObjectIdentifier, value []byte) []byte {
	return seq(oid(o), set(value))
}

// signingCertificateV2 encodes the attribute according to RFC 5035 with the
// hash of the signing certificate and issuer and serial number. PAdES requires
// it (ETSI EN 319 122-1 5.2.2.3, EN 319 142-1 5.2). With SHA-256 the
// algorithm is omitted, it is the default.
func signingCertificateV2(cert *x509.Certificate, hash crypto.Hash) []byte {
	h := hash.New()
	h.Write(cert.Raw)
	essCertID := [][]byte{}
	if hash != crypto.SHA256 {
		essCertID = append(essCertID, algorithm(hashOIDs[hash], false))
	}
	issuerSerial := seq(
		seq(explicit(4, cert.RawIssuer)), // GeneralNames with directoryName
		integer(cert.SerialNumber),
	)
	essCertID = append(essCertID, octets(h.Sum(nil)), issuerSerial)
	return seq(seq(seq(essCertID...)))
}

// SignedOptions controls the structure of SignedData.
type SignedOptions struct {
	// Hash is the hash algorithm, default SHA-256.
	Hash crypto.Hash
	// Detached omits the content (PAdES, ETSI.CAdES.detached), otherwise
	// Content is in the container.
	Detached bool
	// ContentType is the type of the content, default id-data.
	ContentType asn1.ObjectIdentifier
	// Unsigned generates unsigned attributes from the signature, for example the
	// timestamp for PAdES B-T.
	Unsigned func(signature []byte) ([][]byte, error)
}

// SignedData builds a CMS container SignedData (RFC 5652 5) with exactly
// one SignerInfo and the signed attributes content-type,
// message-digest and signing-certificate-v2. signing-time is missing on
// purpose: PAdES forbids it, the time is in /M (ETSI EN 319 142-1
// 5.2.1). content is the content to sign, for PDF the bytes of the
// ByteRange.
func SignedData(ctx context.Context, s Signer, content []byte, opt SignedOptions) ([]byte, error) {
	if opt.Hash == 0 {
		opt.Hash = crypto.SHA256
	}
	if opt.ContentType == nil {
		opt.ContentType = oidData
	}
	hashOID, ok := hashOIDs[opt.Hash]
	if !ok {
		return nil, fmt.Errorf("Hashverfahren %v wird nicht unterstützt", opt.Hash)
	}
	certs := s.Certificates()
	if len(certs) == 0 {
		return nil, errors.New("der Signer hat kein Zertifikat")
	}
	cert := certs[0]
	h := opt.Hash.New()
	h.Write(content)
	signedAttrs := set(
		attribute(oidContentType, oid(opt.ContentType)),
		attribute(oidMessageDigest, octets(h.Sum(nil))),
		attribute(oidSigningCertificateV2, signingCertificateV2(cert, opt.Hash)),
	)
	// The DER encoding as SET is signed (RFC 5652 5.4).
	ah := opt.Hash.New()
	ah.Write(signedAttrs)
	sig, err := s.SignDigest(ctx, opt.Hash, ah.Sum(nil))
	if err != nil {
		return nil, fmt.Errorf("Signieren: %w", err)
	}
	sigAlg, err := signatureAlgorithm(cert.PublicKey, opt.Hash)
	if err != nil {
		return nil, err
	}
	si := [][]byte{
		integer(big.NewInt(1)),
		seq(cert.RawIssuer, integer(cert.SerialNumber)),
		algorithm(hashOID, false),
		implicitCons(0, signedAttrs),
		sigAlg,
		octets(sig),
	}
	if opt.Unsigned != nil {
		attrs, err := opt.Unsigned(sig)
		if err != nil {
			return nil, err
		}
		if len(attrs) > 0 {
			si = append(si, implicitCons(1, set(attrs...)))
		}
	}
	encap := [][]byte{oid(opt.ContentType)}
	if !opt.Detached {
		encap = append(encap, explicit(0, octets(content)))
	}
	var raw [][]byte
	for _, c := range certs {
		raw = append(raw, c.Raw)
	}
	sd := seq(
		integer(big.NewInt(1)),
		set(algorithm(hashOID, false)),
		seq(encap...),
		implicitCons(0, set(raw...)),
		set(seq(si...)),
	)
	return seq(oid(oidSignedData), explicit(0, sd)), nil
}

// TimeStampAttribute encodes the timestamp as an unsigned attribute
// (RFC 3161 Annex A, id-aa-timeStampToken), for PAdES B-T.
func TimeStampAttribute(token []byte) []byte {
	return attribute(oidTimeStampToken, token)
}
