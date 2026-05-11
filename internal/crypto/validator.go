package crypto

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
)

// ValidateSignedContents verifies the signedContents field from a BISS /sign request.
//
// BISS specification for signedContents validation:
//  1. Decode contents[i] from base64 → rawContent
//  2. Compute SHA-256(rawContent) → contentHash
//  3. Decode signedContents[i] from base64 → signature
//  4. Decode signedContentsCert[i] from base64 → parse as X.509
//  5. Verify that signature = RSA-SHA256-sign(contentHash) using cert's public key
//  6. Validate signedContentsCert[i] chains to OS trust store
//
// This proves that the calling server (the web application, not the browser)
// authorised this specific signing request — preventing replay attacks where a
// malicious page could trick the user into signing arbitrary data.
func ValidateSignedContents(contents, signedContents, signedContentsCerts []string) error {
	if len(contents) != len(signedContents) || len(contents) != len(signedContentsCerts) {
		return errors.New("contents, signedContents, and signedContentsCerts must have equal length")
	}

	for i := range contents {
		if err := validateOne(i, contents[i], signedContents[i], signedContentsCerts[i]); err != nil {
			return err
		}
	}

	return nil
}

// validateOne validates a single (content, signature, serverCert) triplet.
func validateOne(idx int, contentB64, signedContentB64, serverCertB64 string) error {
	// Step 1: decode the content bytes.
	rawContent, err := base64.StdEncoding.DecodeString(contentB64)
	if err != nil {
		rawContent, err = base64.RawStdEncoding.DecodeString(contentB64)
		if err != nil {
			return fmt.Errorf("contents[%d]: base64 decode: %w", idx, err)
		}
	}

	// Step 2: SHA-256 hash the content, then hash again.
	// Per BISS spec, signedContents = rsaSha256Sign(sha256(content)).
	// rsaSha256Sign internally hashes with SHA-256 before RSA signing,
	// so the actual signed value is SHA256(SHA256(content)).
	firstHash := sha256.Sum256(rawContent)
	hash := sha256.Sum256(firstHash[:])

	// Step 3: decode the signature.
	sig, err := base64.StdEncoding.DecodeString(signedContentB64)
	if err != nil {
		sig, err = base64.RawStdEncoding.DecodeString(signedContentB64)
		if err != nil {
			return fmt.Errorf("signedContents[%d]: base64 decode: %w", idx, err)
		}
	}

	// Step 4: parse the server certificate.
	serverCert, err := ParseCertBase64(serverCertB64)
	if err != nil {
		return fmt.Errorf("signedContentsCert[%d]: %w", idx, err)
	}

	// Step 5: verify the RSA-PKCS1-v1.5 signature over the SHA-256 hash.
	if err := verifyRSASHA256(serverCert, hash[:], sig); err != nil {
		return fmt.Errorf("signedContents[%d]: signature verification failed: %w", idx, err)
	}

	// Step 6: validate the server cert chains to the OS trust store.
	// We do not pass intermediates here because the server cert should be
	// a full-chain cert or the chain is available in the OS store.
	if err := VerifyChain(serverCert, nil); err != nil {
		return fmt.Errorf("signedContentsCert[%d]: %w", idx, err)
	}

	return nil
}

// verifyRSASHA256 verifies an RSA-SHA256 signature.
// The cert's public key must be an *rsa.PublicKey; ECDSA is not supported by
// the standard BISS implementation.
func verifyRSASHA256(cert *x509.Certificate, hash, sig []byte) error {
	rsaPub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("signedContentsCert has non-RSA public key (%T); only RSA is supported", cert.PublicKey)
	}

	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hash, sig); err != nil {
		return fmt.Errorf("RSA PKCS1v15 verify: %w", err)
	}

	return nil
}
