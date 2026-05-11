// Package crypto provides certificate parsing, chain building, and
// signedContents verification per the BISS /sign request specification.
package crypto

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
)

// ParseCertBase64 decodes a base64-encoded DER certificate and parses it.
// This is used to parse the signerCertificateB64 and signedContentsCert fields
// from BISS API requests.
func ParseCertBase64(b64 string) (*x509.Certificate, error) {
	der, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		// Some clients use URL-safe base64 or omit padding — try a relaxed decoder.
		der, err = base64.RawStdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("base64 decode certificate: %w", err)
		}
	}

	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parse DER certificate: %w", err)
	}

	return cert, nil
}

// BuildPool creates an x509.CertPool from a slice of base64-encoded DER certs.
// Used to build the intermediate pool for chain verification.
func BuildPool(b64Certs []string) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	for i, b64 := range b64Certs {
		cert, err := ParseCertBase64(b64)
		if err != nil {
			return nil, fmt.Errorf("cert[%d]: %w", i, err)
		}
		pool.AddCert(cert)
	}
	return pool, nil
}

// VerifyChain checks that cert chains to a trusted root in the OS certificate store.
// This is the KEY DIFFERENTIATOR vs BISS: we use the OS trust store, not a
// custom B-Trust-only trust store.
//
// intermediates is an optional pool of intermediate CA certificates to help
// complete the chain when they are not present in the OS store.
func VerifyChain(cert *x509.Certificate, intermediates *x509.CertPool) error {
	// x509.SystemCertPool() returns the OS trust store:
	//   - macOS: Security framework (Keychain)
	//   - Windows: Windows Certificate Store
	//   - Linux: /etc/ssl/certs or equivalent
	roots, err := x509.SystemCertPool()
	if err != nil {
		// SystemCertPool can fail on some restricted environments.
		// Fall back to an empty pool which will reject self-signed certs
		// but still validate properly-chained ones if intermediates are provided.
		roots = x509.NewCertPool()
	}

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
		// KeyUsages: we don't restrict to a specific usage here because
		// the signedContentsCert is a TLS certificate (server auth), not
		// necessarily a code signing cert.
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
	}

	_, err = cert.Verify(opts)
	if err != nil {
		return fmt.Errorf("certificate chain validation failed: %w", err)
	}

	return nil
}
