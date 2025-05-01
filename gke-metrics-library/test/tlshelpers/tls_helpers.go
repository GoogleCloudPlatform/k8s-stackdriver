// Package tlshelpers provides tools to generate TLS certificates for testing.
package tlshelpers

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	
	"k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
)

// NewTestCAPool with the specified certificate.
func NewTestCAPool(cert *x509.Certificate) *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return pool
}

// NewTestCAFile stores the raw certificate in the test temp directory
// and returns the name.
func NewTestCAFile(t *testing.T, raw []byte) (string, error) {
	caCertFile, err := os.CreateTemp(t.TempDir(), "*-ca.crt")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(caCertFile.Name(), raw, 0644); err != nil {
		return "", err
	}

	return caCertFile.Name(), nil
}

// EncodeCert certificate into raw bytes.
func EncodeCert(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

// NewTestCA returns a self signed CA cert that can be used for testing.
func NewTestCA() (*x509.Certificate, error) {
	cert, _, err := NewTestCAAndKey()
	return cert, err
}

// NewTestCAAndKey returns a self signed CA cert and key that can be used for testing.
func NewTestCAAndKey() (*x509.Certificate, *rsa.PrivateKey, error) {
	signingKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	signingCert, err := cert.NewSelfSignedCACert(
		cert.Config{CommonName: "gke-metrics-test"},
		signingKey,
	)
	if err != nil {
		return nil, nil, err
	}

	return signingCert, signingKey, nil
}

// UpdateCAFile with new content.
func UpdateCAFile(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	if _, err := file.Write(raw); err != nil {
		return err
	}
	return file.Close()
}

// NewTestSelfSignedCertificate using the signing certificate.
// Returns the certificate and its key.
func NewTestSelfSignedCertificate(signingCert *x509.Certificate, signingKey crypto.Signer) ([]byte, []byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).SetInt64(math.MaxInt64))
	if err != nil {
		return nil, nil, err
	}

	certTmpl := &x509.Certificate{
		Subject: pkix.Name{
			CommonName: "gke-metrics-test",
		},
		SerialNumber: serial,
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:    signingCert.NotBefore,
		NotAfter:     time.Now().Add(time.Hour * 24).UTC(),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDERBytes, err := x509.CreateCertificate(rand.Reader, certTmpl, signingCert, key.Public(), signingKey)
	if err != nil {
		return nil, nil, err
	}

	keyPEM, err := keyutil.MarshalPrivateKeyToPEM(key)
	if err != nil {
		return nil, nil, err
	}

	certParsed, err := x509.ParseCertificate(certDERBytes)
	if err != nil {
		return nil, nil, err
	}

	return EncodeCert(certParsed), keyPEM, nil
}
