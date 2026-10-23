package sign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/x509"
	"encoding/asn1"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"
)

// OIDTSTInfo is the content type of a timestamp (RFC 3161 2.4.2).
var OIDTSTInfo = oidTSTInfo

// TSA requests timestamps according to RFC 3161 from a service over HTTP
// (RFC 3161 3.4, media type application/timestamp-query).
type TSA struct {
	URL string
	// Client is the HTTP client, default with a 30 second timeout.
	Client *http.Client
	// Hash is the algorithm for the imprint, default SHA-256.
	Hash crypto.Hash
}

// maxTSAResponse limits the response of a timestamp service.
const maxTSAResponse = 1 << 20

// Timestamp returns a timestamp over data as a CMS ContentInfo. It
// checks that the imprint and nonce of the response match the request.
func (t *TSA) Timestamp(ctx context.Context, data []byte) ([]byte, error) {
	hash := t.Hash
	if hash == 0 {
		hash = crypto.SHA256
	}
	h := hash.New()
	h.Write(data)
	imprint := h.Sum(nil)
	nonce, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 63))
	if err != nil {
		return nil, err
	}
	req := seq(
		integer(big.NewInt(1)),
		seq(algorithm(hashOIDs[hash], false), octets(imprint)),
		integer(nonce),
		[]byte{0x01, 0x01, 0xff}, // certReq TRUE: include the certificate of the service
	)
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, t.URL, bytes.NewReader(req))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/timestamp-query")
	resp, err := client.Do(hr)
	if err != nil {
		return nil, fmt.Errorf("Zeitstempeldienst: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxTSAResponse))
	if err != nil {
		return nil, fmt.Errorf("Zeitstempeldienst: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Zeitstempeldienst antwortet mit %s", resp.Status)
	}
	token, err := parseTimeStampResp(body)
	if err != nil {
		return nil, err
	}
	info, err := ParseTimeStamp(token)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(info.Imprint, imprint) {
		return nil, errors.New("Zeitstempel: der Abdruck passt nicht zur Anfrage")
	}
	if info.Nonce == nil || info.Nonce.Cmp(nonce) != 0 {
		return nil, errors.New("Zeitstempel: die Nonce passt nicht zur Anfrage")
	}
	return token, nil
}

type pkiStatusInfo struct {
	Status       int
	StatusString asn1.RawValue  `asn1:"optional"`
	FailInfo     asn1.BitString `asn1:"optional"`
}

type timeStampResp struct {
	Status pkiStatusInfo
	Token  asn1.RawValue `asn1:"optional"`
}

// parseTimeStampResp returns the token of a successful response.
func parseTimeStampResp(der []byte) ([]byte, error) {
	var r timeStampResp
	rest, err := asn1.Unmarshal(der, &r)
	if err != nil || len(rest) > 0 {
		return nil, fmt.Errorf("Zeitstempel: Antwort nicht lesbar")
	}
	// granted (0) or grantedWithMods (1), RFC 3161 2.4.2.
	if r.Status.Status > 1 {
		return nil, fmt.Errorf("Zeitstempel abgelehnt, Status %d", r.Status.Status)
	}
	if len(r.Token.FullBytes) == 0 {
		return nil, errors.New("Zeitstempel: Antwort ohne Token")
	}
	return r.Token.FullBytes, nil
}

// TimeStampInfo are the attributes of a timestamp.
type TimeStampInfo struct {
	Time    time.Time
	Imprint []byte
	Hash    asn1.ObjectIdentifier
	Nonce   *big.Int
	Serial  *big.Int
	// Certificates are the certificates in the token, that of the service first,
	// if included.
	Certificates []*x509.Certificate
}

type messageImprint struct {
	Alg  algorithmIdentifier
	Hash []byte
}

type algorithmIdentifier struct {
	Algorithm  asn1.ObjectIdentifier
	Parameters asn1.RawValue `asn1:"optional"`
}

// accuracy is a SEQUENCE (RFC 3161 2.4.2). As a RawValue it would take up the
// following nonce if it were absent.
type accuracy struct {
	Seconds int `asn1:"optional"`
	Millis  int `asn1:"optional,tag:0"`
	Micros  int `asn1:"optional,tag:1"`
}

type tstInfo struct {
	Version   int
	Policy    asn1.ObjectIdentifier
	Imprint   messageImprint
	Serial    *big.Int
	GenTime   time.Time     `asn1:"generalized"`
	Accuracy  accuracy      `asn1:"optional"`
	Ordering  bool          `asn1:"optional,default:false"`
	Nonce     *big.Int      `asn1:"optional"`
	TSA       asn1.RawValue `asn1:"optional,tag:0"`
	Extension asn1.RawValue `asn1:"optional,tag:1"`
}

// ParseTimeStamp reads a timestamp token.
func ParseTimeStamp(token []byte) (*TimeStampInfo, error) {
	sd, err := ParseSignedData(token)
	if err != nil {
		return nil, fmt.Errorf("Zeitstempel: %w", err)
	}
	if !sd.ContentType.Equal(oidTSTInfo) {
		return nil, errors.New("Zeitstempel: Inhalt ist kein TSTInfo")
	}
	var ti tstInfo
	if _, err := asn1.Unmarshal(sd.Content, &ti); err != nil {
		return nil, fmt.Errorf("Zeitstempel: TSTInfo nicht lesbar: %v", err)
	}
	return &TimeStampInfo{Time: ti.GenTime, Imprint: ti.Imprint.Hash, Hash: ti.Imprint.Alg.Algorithm,
		Nonce: ti.Nonce, Serial: ti.Serial, Certificates: sd.Certificates}, nil
}

// SignedDataInfo are the parts of a CMS SignedData that pdx needs.
type SignedDataInfo struct {
	ContentType  asn1.ObjectIdentifier
	Content      []byte
	Certificates []*x509.Certificate
	// SignerInfos are the DER encodings of the SignerInfo entries.
	SignerInfos [][]byte
}

type contentInfo struct {
	Type    asn1.ObjectIdentifier
	Content asn1.RawValue `asn1:"explicit,tag:0"`
}

type encapContentInfo struct {
	Type    asn1.ObjectIdentifier
	Content asn1.RawValue `asn1:"optional,explicit,tag:0"`
}

type signedData struct {
	Version     int
	DigestAlgs  asn1.RawValue
	Encap       encapContentInfo
	Certs       asn1.RawValue `asn1:"optional,tag:0"`
	CRLs        asn1.RawValue `asn1:"optional,tag:1"`
	SignerInfos asn1.RawValue
}

// ParseSignedData reads a CMS ContentInfo with SignedData (RFC 5652).
func ParseSignedData(der []byte) (*SignedDataInfo, error) {
	var ci contentInfo
	if _, err := asn1.Unmarshal(der, &ci); err != nil {
		return nil, fmt.Errorf("CMS nicht lesbar: %v", err)
	}
	if !ci.Type.Equal(oidSignedData) {
		return nil, errors.New("CMS enthält kein SignedData")
	}
	var sd signedData
	if _, err := asn1.Unmarshal(ci.Content.Bytes, &sd); err != nil {
		return nil, fmt.Errorf("SignedData nicht lesbar: %v", err)
	}
	out := &SignedDataInfo{ContentType: sd.Encap.Type}
	if len(sd.Encap.Content.Bytes) > 0 {
		var oct []byte
		if _, err := asn1.Unmarshal(sd.Encap.Content.Bytes, &oct); err != nil {
			return nil, fmt.Errorf("Inhalt von SignedData nicht lesbar: %v", err)
		}
		out.Content = oct
	}
	if len(sd.Certs.Bytes) > 0 {
		certs, err := x509.ParseCertificates(sd.Certs.Bytes)
		if err != nil {
			return nil, fmt.Errorf("Zertifikate in SignedData: %v", err)
		}
		out.Certificates = certs
	}
	for rest := sd.SignerInfos.Bytes; len(rest) > 0; {
		var raw asn1.RawValue
		var err error
		if rest, err = asn1.Unmarshal(rest, &raw); err != nil {
			return nil, fmt.Errorf("SignerInfo nicht lesbar: %v", err)
		}
		out.SignerInfos = append(out.SignerInfos, raw.FullBytes)
	}
	return out, nil
}
