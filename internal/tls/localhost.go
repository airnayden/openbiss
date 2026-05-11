// Package tls provides self-signed TLS certificate generation and loading for
// the OpenBISS localhost HTTPS server.
//
// BISS ships with a bundled certificate signed by the B-Trust CA. OpenBISS
// instead generates a fresh self-signed certificate on first run and reuses
// it across restarts, storing it in the user's data directory.
//
// The generated certificate covers:
//   - IP SANs: 127.0.0.1, ::1
//   - DNS SANs: localhost
//
// Clients (browsers calling the BISS API) typically already ignore cert errors
// for BISS because BISS uses a custom B-Trust CA that is not in the OS trust store.
// OpenBISS behaves identically from the browser's perspective.
package tls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/openbiss/openbiss/internal/i18n"
)

// LoadOrGenerate loads TLS credentials from certPath/keyPath, generating a new
// self-signed certificate when either file is missing or cannot be parsed.
func LoadOrGenerate(certPath, keyPath string) (tls.Certificate, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err == nil {
		slog.Info(i18n.T("tls.loaded_cert"), "cert", certPath)
		return cert, nil
	}

	slog.Info(i18n.T("tls.generating_cert"), "cert", certPath)
	if err := generate(certPath, keyPath); err != nil {
		return tls.Certificate{}, fmt.Errorf("generate self-signed cert: %w", err)
	}

	trustCert(certPath)

	return tls.LoadX509KeyPair(certPath, keyPath)
}

// generate creates a new ECDSA P-256 self-signed certificate valid for 10 years
// and writes the PEM-encoded certificate and key to certPath and keyPath.
func generate(certPath, keyPath string) error {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	// Serial number is a random 128-bit integer for uniqueness.
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("generate serial: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization:       []string{"OpenBISS"},
			OrganizationalUnit: []string{"Self-Signed"},
			CommonName:         "localhost",
		},
		NotBefore:             now.Add(-time.Minute), // small back-date to handle clock skew
		NotAfter:              now.Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	if err := writePEM(certPath, "CERTIFICATE", certDER); err != nil {
		return err
	}

	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}

	return writePEM(keyPath, "EC PRIVATE KEY", privDER)
}

// writePEM writes a single PEM block with the given type and DER bytes to path.
// The file is created with mode 0600 so the private key is not world-readable.
func writePEM(path, pemType string, der []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	return pem.Encode(f, &pem.Block{Type: pemType, Bytes: der})
}

// NewTLSConfig returns a *tls.Config suitable for the OpenBISS HTTPS server.
// It enforces TLS 1.2 minimum (matching modern browser requirements) and uses
// the provided certificate.
func NewTLSConfig(cert tls.Certificate) *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		// Prefer server cipher suites so we have some control over negotiated suites.
		PreferServerCipherSuites: true, //nolint:staticcheck // intentional for compatibility
	}
}
