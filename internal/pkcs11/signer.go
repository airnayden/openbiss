package pkcs11

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	p11 "github.com/miekg/pkcs11"

	"github.com/airnayden/openbiss/internal/i18n"
)

// ErrPrivKeyNotFound is returned when no CKO_PRIVATE_KEY matching the
// certificate's public key can be found on the token.
var ErrPrivKeyNotFound = errors.New("private key not found on token")

// Signer performs PKCS#11 signing operations.
// It wraps Driver and adds PIN-protected session login and signing.
type Signer struct {
	driver *Driver
	mu     sync.Mutex
}

// NewSigner creates a Signer that uses the provided Driver.
func NewSigner(d *Driver) *Signer {
	return &Signer{driver: d}
}

// Sign signs data using the private key corresponding to cert.
// The PIN is used to log in to the token; it is wiped from memory after use.
//
// Mechanism: CKM_SHA256_RSA_PKCS — the PKCS#11 module performs the full
// SHA-256 + PKCS#1 v1.5 padding operation internally, which is what BISS
// expects in the /sign response.
//
// The BISS /sign endpoint's signedContents field contains a SHA-256 hash of
// the content already signed by the server, but the actual token signing
// (CKM_SHA256_RSA_PKCS) re-hashes internally — we pass the raw content bytes.
//
// On session-invalidation errors the driver automatically re-initialises and
// retries the operation once.
func (s *Signer) Sign(cert *CertWithSlot, data []byte, pin []byte) ([]byte, error) {
	var sig []byte
	err := s.driver.withSessionRetry(func() error {
		var e error
		sig, e = s.doSign(cert, data, pin)
		return e
	})
	return sig, err
}

// doSign is the internal implementation that acquires s.mu and d.mu, opens a
// PKCS#11 session, logs in, and performs the signing operation. Both mutexes
// are released via defer before doSign returns so that withSessionRetry can
// call reInit() without deadlocking on d.mu.
func (s *Signer) doSign(cert *CertWithSlot, data []byte, pin []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	d := s.driver
	d.mu.Lock()
	defer d.mu.Unlock()

	// Open a read/write session needed for C_Login.
	session, err := d.ctx.OpenSession(cert.SlotID, p11.CKF_SERIAL_SESSION|p11.CKF_RW_SESSION)
	if err != nil {
		return nil, fmt.Errorf("open signing session on slot %d: %w", cert.SlotID, err)
	}
	defer func() {
		_ = d.ctx.Logout(session)
		_ = d.ctx.CloseSession(session)
	}()

	// SECURITY: only allowed string() conversion — miekg/pkcs11 API requires string
	err = d.ctx.Login(session, p11.CKU_USER, string(pin))
	if err != nil {
		if pkcs11Err, ok := err.(p11.Error); ok {
			switch pkcs11Err {
			case p11.CKR_PIN_INCORRECT:
				return nil, errors.New(i18n.T("error.incorrect_pin"))
			case p11.CKR_PIN_LOCKED:
				return nil, errors.New(i18n.T("error.pin_locked"))
			case p11.CKR_USER_ALREADY_LOGGED_IN:
				// Already logged in (e.g., another application). Proceed.
				slog.Debug("PKCS#11 user already logged in", "slot", cert.SlotID)
			default:
				return nil, fmt.Errorf("PKCS#11 Login: %w", err)
			}
		} else {
			return nil, fmt.Errorf("PKCS#11 Login: %w", err)
		}
	}

	privKeyHandle, err := s.findPrivateKey(d, session, cert)
	if err != nil {
		return nil, err
	}

	// CKM_SHA256_RSA_PKCS: the module does SHA-256 + PKCS#1 v1.5 sign internally.
	mechanism := []*p11.Mechanism{p11.NewMechanism(p11.CKM_SHA256_RSA_PKCS, nil)}
	if err := d.ctx.SignInit(session, mechanism, privKeyHandle); err != nil {
		return nil, fmt.Errorf("SignInit: %w", err)
	}

	sig, err := d.ctx.Sign(session, data)
	if err != nil {
		return nil, fmt.Errorf("Sign: %w", err)
	}

	slog.Debug("signing complete", "sigLen", len(sig))
	return sig, nil
}

// findPrivateKey locates the CKO_PRIVATE_KEY object that corresponds to cert
// by matching CKA_ID attributes between the certificate and key objects.
func (s *Signer) findPrivateKey(d *Driver, session p11.SessionHandle, cert *CertWithSlot) (p11.ObjectHandle, error) {
	// First, get the CKA_ID of the certificate object.
	attrs, err := d.ctx.GetAttributeValue(session, cert.ObjectHandle, []*p11.Attribute{
		p11.NewAttribute(p11.CKA_ID, nil),
	})
	if err != nil {
		return 0, fmt.Errorf("get CKA_ID from cert: %w", err)
	}

	var certID []byte
	for _, attr := range attrs {
		if attr.Type == p11.CKA_ID {
			certID = attr.Value
		}
	}

	// Search for a CKO_PRIVATE_KEY with the same CKA_ID.
	template := []*p11.Attribute{
		p11.NewAttribute(p11.CKA_CLASS, p11.CKO_PRIVATE_KEY),
	}
	if len(certID) > 0 {
		template = append(template, p11.NewAttribute(p11.CKA_ID, certID))
	}

	if err := d.ctx.FindObjectsInit(session, template); err != nil {
		return 0, fmt.Errorf("FindObjectsInit for private key: %w", err)
	}
	defer func() { _ = d.ctx.FindObjectsFinal(session) }()

	handles, _, err := d.ctx.FindObjects(session, 1)
	if err != nil {
		return 0, fmt.Errorf("FindObjects for private key: %w", err)
	}
	if len(handles) == 0 {
		return 0, ErrPrivKeyNotFound
	}

	return handles[0], nil
}
