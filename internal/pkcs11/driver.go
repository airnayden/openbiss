package pkcs11

import (
	"crypto/x509"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	p11 "github.com/miekg/pkcs11"

	"github.com/openbiss/openbiss/internal/i18n"
)

// ErrNoToken is returned when no PKCS#11 token/reader is available.
var ErrNoToken = errors.New("no PKCS#11 token found")

// ErrNoCerts is returned when a token is found but contains no certificates.
var ErrNoCerts = errors.New("no certificates found on token")

// CertWithSlot groups an x509 certificate with its PKCS#11 location so the
// signer can later open the correct session to retrieve the private key handle.
type CertWithSlot struct {
	// Certificate is the parsed X.509 certificate.
	Certificate *x509.Certificate

	// SlotID is the PKCS#11 slot index where this certificate resides.
	SlotID uint

	// ObjectHandle is the CKO_CERTIFICATE object handle within the slot.
	ObjectHandle p11.ObjectHandle

	// Label is the human-readable token/object label shown in selection dialogs.
	Label string

	// RawDER is the raw DER-encoded certificate bytes, used for base64 responses.
	RawDER []byte
}

// Driver manages a PKCS#11 module (shared library) lifecycle.
// It is safe for concurrent use; the internal mutex serialises PKCS#11 calls
// which are not guaranteed thread-safe by the specification.
type Driver struct {
	mu            sync.Mutex
	ctx           *p11.Ctx
	lib           string
	sessionLostCb atomic.Pointer[func()]
}

// NewDriver loads the PKCS#11 shared library at libPath and initialises it.
// The caller must call Close when done to release the module handle.
func NewDriver(libPath string) (*Driver, error) {
	ctx := p11.New(libPath)
	if err := ctx.Initialize(); err != nil {
		// CKR_CRYPTOKI_ALREADY_INITIALIZED is benign — another process may
		// have already initialised the module (e.g., a browser plugin).
		if pkcs11Err, ok := err.(p11.Error); ok && pkcs11Err == p11.CKR_CRYPTOKI_ALREADY_INITIALIZED {
			slog.Debug("PKCS#11 module already initialised", "lib", libPath)
		} else {
			ctx.Destroy()
			return nil, fmt.Errorf("initialize PKCS#11 module %s: %w", libPath, err)
		}
	}

	slog.Info(i18n.T("pkcs11.module_loaded"), "lib", libPath)
	return &Driver{ctx: ctx, lib: libPath}, nil
}

// LibPath returns the absolute filesystem path of the PKCS#11 shared
// library this driver was constructed with. Useful for status displays
// and diagnostics. The value is set once in NewDriver and never mutates
// thereafter, so no locking is required.
func (d *Driver) LibPath() string {
	return d.lib
}

// Close finalises and unloads the PKCS#11 module.
// The mutex is released BEFORE calling ctx.Finalize() to eliminate the
// deadlock window where withSessionRetry or reInit could block on d.mu
// while Close holds it during a slow Finalize call.
func (d *Driver) Close() {
	d.mu.Lock()
	ctx := d.ctx
	d.ctx = nil
	d.mu.Unlock()

	if ctx == nil {
		return
	}
	_ = ctx.Finalize()
	ctx.Destroy()
}

// OnSessionLost registers a callback that is invoked (in a new goroutine)
// whenever the driver automatically re-initialises the PKCS#11 session after
// a CKR_SESSION_HANDLE_INVALID / CKR_TOKEN_NOT_PRESENT / CKR_DEVICE_REMOVED /
// CKR_CRYPTOKI_NOT_INITIALIZED error. Calling this again replaces the previous
// callback. Pass nil to deregister.
func (d *Driver) OnSessionLost(fn func()) {
	if fn == nil {
		d.sessionLostCb.Store(nil)
		return
	}
	d.sessionLostCb.Store(&fn)
}

// isSessionError reports whether err is a PKCS#11 error that indicates the
// current session or context is no longer usable (e.g., after a sleep/wake
// cycle or token removal).
func isSessionError(err error) bool {
	var p11Err p11.Error
	if !errors.As(err, &p11Err) {
		return false
	}
	switch p11Err {
	case p11.CKR_SESSION_HANDLE_INVALID,
		p11.CKR_TOKEN_NOT_PRESENT,
		p11.CKR_DEVICE_REMOVED,
		p11.CKR_CRYPTOKI_NOT_INITIALIZED:
		return true
	}
	return false
}

// reInit creates a fresh PKCS#11 context for d.lib, atomically replaces
// d.ctx, and finalises the old context outside the mutex to avoid deadlocks.
func (d *Driver) reInit() error {
	ctx := p11.New(d.lib)
	if err := ctx.Initialize(); err != nil {
		if pkcs11Err, ok := err.(p11.Error); ok && pkcs11Err == p11.CKR_CRYPTOKI_ALREADY_INITIALIZED {
			slog.Debug("PKCS#11 module already initialised on reinit", "lib", d.lib)
		} else {
			ctx.Destroy()
			return fmt.Errorf("reinitialize PKCS#11 module %s: %w", d.lib, err)
		}
	}

	d.mu.Lock()
	old := d.ctx
	d.ctx = ctx
	d.mu.Unlock()

	// Finalize and destroy the old context OUTSIDE the mutex.
	if old != nil {
		_ = old.Finalize()
		old.Destroy()
	}

	return nil
}

// withSessionRetry runs op and, if it returns a session-invalidation error,
// transparently re-initialises the PKCS#11 context and retries op exactly once.
// If the retry also fails the retry error is returned. If re-initialisation
// itself fails the original error is returned (the module state is unknown).
// The registered OnSessionLost callback is invoked asynchronously on success.
func (d *Driver) withSessionRetry(op func() error) error {
	err := op()
	if err == nil {
		return nil
	}
	if !isSessionError(err) {
		return err
	}

	slog.Warn("PKCS#11 session invalidated; reinitializing", "error", err)
	if reInitErr := d.reInit(); reInitErr != nil {
		// Re-init failed; return the original error so the caller sees what
		// triggered the recovery attempt.
		return err
	}

	if cb := d.sessionLostCb.Load(); cb != nil {
		go (*cb)()
	}

	return op() // retry once
}

// ListCertificates returns all X.509 certificates found across all token slots.
// It skips slots that have no token present (CKF_TOKEN_PRESENT not set).
// On session-invalidation errors the driver automatically re-initialises and
// retries the operation once.
func (d *Driver) ListCertificates() ([]*CertWithSlot, error) {
	var certs []*CertWithSlot
	err := d.withSessionRetry(func() error {
		var e error
		certs, e = d.listCertificates()
		return e
	})
	return certs, err
}

// listCertificates is the internal (mutex-acquiring) implementation used by
// ListCertificates via withSessionRetry. It acquires and releases d.mu so
// that withSessionRetry can call reInit() if the operation fails with a
// session error.
func (d *Driver) listCertificates() ([]*CertWithSlot, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	slots, err := d.ctx.GetSlotList(true) // true = only slots with token present
	if err != nil {
		return nil, fmt.Errorf("get slot list: %w", err)
	}

	if len(slots) == 0 {
		return nil, ErrNoToken
	}

	var certs []*CertWithSlot
	for _, slot := range slots {
		slotCerts, err := d.listCertsInSlot(slot)
		if err != nil {
			slog.Warn("failed to list certs in slot", "slot", slot, "error", err)
			continue
		}
		certs = append(certs, slotCerts...)
	}

	if len(certs) == 0 {
		return nil, ErrNoCerts
	}

	return certs, nil
}

// listCertsInSlot opens a read-only session on slot, enumerates CKO_CERTIFICATE
// objects, parses each as X.509, and returns the results.
// Callers must hold d.mu.
func (d *Driver) listCertsInSlot(slot uint) ([]*CertWithSlot, error) {
	session, err := d.ctx.OpenSession(slot, p11.CKF_SERIAL_SESSION)
	if err != nil {
		return nil, fmt.Errorf("open session on slot %d: %w", slot, err)
	}
	defer func() { _ = d.ctx.CloseSession(session) }()

	// Retrieve the token label for display in selection dialogs.
	tokenInfo, err := d.ctx.GetTokenInfo(slot)
	tokenLabel := "Unknown"
	if err == nil {
		tokenLabel = tokenInfo.Label
	}

	// Search for all certificate objects (CKO_CERTIFICATE).
	certClass := p11.NewAttribute(p11.CKA_CLASS, p11.CKO_CERTIFICATE)
	if err := d.ctx.FindObjectsInit(session, []*p11.Attribute{certClass}); err != nil {
		return nil, fmt.Errorf("FindObjectsInit: %w", err)
	}
	defer func() { _ = d.ctx.FindObjectsFinal(session) }()

	var certs []*CertWithSlot
	for {
		handles, _, err := d.ctx.FindObjects(session, 10)
		if err != nil {
			return nil, fmt.Errorf("FindObjects: %w", err)
		}
		if len(handles) == 0 {
			break
		}

		for _, handle := range handles {
			cert, err := d.parseCertObject(session, handle, slot, tokenLabel)
			if err != nil {
				slog.Warn("failed to parse certificate object", "slot", slot, "handle", handle, "error", err)
				continue
			}
			certs = append(certs, cert)
		}
	}

	return certs, nil
}

// parseCertObject reads the CKA_VALUE attribute (DER-encoded cert) from a
// CKO_CERTIFICATE object and parses it into an x509.Certificate.
// Callers must hold d.mu.
func (d *Driver) parseCertObject(session p11.SessionHandle, handle p11.ObjectHandle, slot uint, tokenLabel string) (*CertWithSlot, error) {
	attrs, err := d.ctx.GetAttributeValue(session, handle, []*p11.Attribute{
		p11.NewAttribute(p11.CKA_VALUE, nil),
		p11.NewAttribute(p11.CKA_LABEL, nil),
	})
	if err != nil {
		return nil, fmt.Errorf("GetAttributeValue: %w", err)
	}

	var derBytes []byte
	objectLabel := tokenLabel
	for _, attr := range attrs {
		switch attr.Type {
		case p11.CKA_VALUE:
			derBytes = attr.Value
		case p11.CKA_LABEL:
			if len(attr.Value) > 0 {
				objectLabel = string(attr.Value)
			}
		}
	}

	if len(derBytes) == 0 {
		return nil, errors.New("empty CKA_VALUE")
	}

	// Some PKCS#11 implementations wrap the DER cert in another ASN.1 layer.
	// Attempt to unwrap if parsing as plain DER fails.
	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		unwrapped, unwrapErr := unwrapCertValue(derBytes)
		if unwrapErr != nil {
			return nil, fmt.Errorf("parse certificate DER: %w", err)
		}
		cert, err = x509.ParseCertificate(unwrapped)
		if err != nil {
			return nil, fmt.Errorf("parse unwrapped certificate DER: %w", err)
		}
		derBytes = unwrapped
	}

	return &CertWithSlot{
		Certificate:  cert,
		SlotID:       slot,
		ObjectHandle: handle,
		Label:        objectLabel,
		RawDER:       derBytes,
	}, nil
}

// unwrapCertValue attempts to decode a double-wrapped ASN.1 OCTET STRING
// that some PKCS#11 implementations use for the CKA_VALUE attribute.
func unwrapCertValue(data []byte) ([]byte, error) {
	var inner []byte
	if _, err := asn1.Unmarshal(data, &inner); err != nil {
		return nil, err
	}
	return inner, nil
}

// BuildChain constructs the certificate chain for the given end-entity cert
// from the certs available on the same token, returning DER-encoded PEM blocks
// ordered leaf → intermediate → root.
func BuildChain(leaf *CertWithSlot, all []*CertWithSlot) [][]byte {
	chain := [][]byte{leaf.RawDER}

	// Simple greedy chain builder: follow IssuedBy relationships up the tree.
	current := leaf.Certificate
	for {
		if current.IsCA && current.Issuer.String() == current.Subject.String() {
			// Self-signed root — stop here; it's already appended below.
			break
		}

		parent := findIssuer(current, all)
		if parent == nil {
			break
		}
		chain = append(chain, parent.RawDER)
		current = parent.Certificate

		// Avoid infinite loops if the token has a corrupt/circular chain.
		if len(chain) > 10 {
			break
		}
	}

	return chain
}

// findIssuer searches all for the certificate that issued cert.
func findIssuer(cert *x509.Certificate, all []*CertWithSlot) *CertWithSlot {
	for _, candidate := range all {
		if candidate.Certificate.Subject.String() == cert.Issuer.String() &&
			candidate.Certificate.SerialNumber.Cmp(cert.SerialNumber) != 0 {
			return candidate
		}
	}
	return nil
}

// DERToPEM converts DER-encoded certificate bytes to a PEM block string.
// Useful for debug output; not used in API responses (which use raw base64 DER).
func DERToPEM(der []byte) string {
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}
