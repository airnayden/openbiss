package server

import (
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	bisscrpyto "github.com/airnayden/openbiss/internal/crypto"
	"github.com/airnayden/openbiss/internal/i18n"
	"github.com/airnayden/openbiss/internal/pkcs11"
)

// ---- BISS-compatible request/response types ----

// versionResponse is the exact JSON the BISS /version endpoint returns.
// Browsers and integration clients parse this to detect BISS presence and
// capabilities. All fields must match exactly.
type versionResponse struct {
	Version        string `json:"version"`
	HTTPMethods    string `json:"httpMethods"`
	ContentTypes   string `json:"contentTypes"`
	SignatureTypes string `json:"signatureTypes"`
	SelectorAvail  bool   `json:"selectorAvailable"`
	HashAlgorithms string `json:"hashAlgorithms"`
}

// getSignerRequest is the JSON body sent to /getsigner.
type getSignerRequest struct {
	Selector       map[string]interface{} `json:"selector"`
	ShowValidCerts bool                   `json:"showValidCerts"`
}

// getSignerResponse is the JSON body returned by /getsigner on success.
// Chain is an ordered slice of base64-DER certificates: [leaf, intermediate*, root?].
type getSignerResponse struct {
	Status     string   `json:"status"`
	ReasonCode string   `json:"reasonCode"`
	ReasonText string   `json:"reasonText"`
	Chain      []string `json:"chain"`
}

// signRequest is the JSON body sent to /sign.
type signRequest struct {
	Version              string   `json:"version"`
	SignerCertificateB64 string   `json:"signerCertificateB64"`
	Contents             []string `json:"contents"`
	SignedContents       []string `json:"signedContents"`
	SignedContentsCerts  []string `json:"signedContentsCert"`
	ContentType          string   `json:"contentType"`
	HashAlgorithm        string   `json:"hashAlgorithm"`
	SignatureType        string   `json:"signatureType"`
	ConfirmText          []string `json:"confirmText"`
}

// signResponse is the JSON body returned by /sign on success.
type signResponse struct {
	Status             string   `json:"status"`
	ReasonCode         string   `json:"reasonCode"`
	Signatures         []string `json:"signatures"`
	SignatureType      string   `json:"signatureType"`
	SignatureAlgorithm string   `json:"signatureAlgorithm"`
}

// errorResponse is returned for all error conditions, using the same field
// names as BISS so existing error-handling code in client applications works.
type errorResponse struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reasonCode"`
	ReasonText string `json:"reasonText"`
}

// ---- Handlers ----

// handleVersion responds to GET /version with the fixed BISS capability document.
// Browsers use this response to detect that a BISS-compatible service is running
// and to discover supported hash algorithms.
func handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "405", i18n.T("error.method_not_allowed"))
		return
	}

	writeJSON(w, http.StatusOK, versionResponse{
		Version:        "1.0",
		HTTPMethods:    "GET, POST",
		ContentTypes:   "data, digest",
		SignatureTypes: "signature",
		SelectorAvail:  true,
		HashAlgorithms: "SHA1, SHA256, SHA384, SHA512",
	})
}

// handleGetSigner enumerates smart card certificates via PKCS#11, optionally
// shows a selection dialog if multiple certs are found, and returns the chosen
// certificate's chain as base64-DER strings.
func (s *Server) handleGetSigner(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "405", i18n.T("error.method_not_allowed"))
		return
	}

	var req getSignerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "400", i18n.T("error.invalid_json"))
		return
	}

	driver := s.Driver()
	if driver == nil {
		writeError(w, http.StatusServiceUnavailable, "503", i18n.T("pkcs11.no_libraries"))
		return
	}

	certs, err := driver.ListCertificates()
	if err != nil {
		slog.Error("list certificates failed", "error", err)
		writeError(w, http.StatusInternalServerError, "500", i18n.T("pkcs11.no_certs", err))
		return
	}

	options := make([]string, len(certs))
	for i, c := range certs {
		options[i] = i18n.T("dialog.cert_format",
			c.Certificate.Subject.CommonName,
			c.Certificate.Issuer.CommonName,
		)
	}

	idx, err := s.dialog.SelectCertificate(i18n.T("dialog.cert_title"), options)
	if err != nil {
		slog.Error("certificate selection failed", "error", err)
		writeError(w, http.StatusInternalServerError, "500", i18n.T("error.cert_selection_failed"))
		return
	}
	chosen := certs[idx]

	// Build the certificate chain: leaf → intermediates (from the same token).
	chainDER := pkcs11.BuildChain(chosen, certs)
	chainB64 := make([]string, len(chainDER))
	for i, der := range chainDER {
		chainB64[i] = base64.StdEncoding.EncodeToString(der)
	}

	writeJSON(w, http.StatusOK, getSignerResponse{
		Status:     "ok",
		ReasonCode: "200",
		ReasonText: i18n.T("api.success"),
		Chain:      chainB64,
	})
}

// handleSign verifies the signedContents authorisation proof, locates the
// matching private key on the smart card, prompts for PIN, and signs each
// content item using CKM_SHA256_RSA_PKCS.
func (s *Server) handleSign(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "405", i18n.T("error.method_not_allowed"))
		return
	}

	var req signRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "400", i18n.T("error.invalid_json"))
		return
	}

	if req.SignerCertificateB64 == "" {
		writeError(w, http.StatusBadRequest, "400", i18n.T("error.signer_cert_required"))
		return
	}
	if len(req.Contents) == 0 {
		writeError(w, http.StatusBadRequest, "400", i18n.T("error.contents_required"))
		return
	}

	// Verify signedContents — proves the calling server authorised this request.
	// This is the BISS anti-replay mechanism. We validate against the OS trust
	// store, not BISS's custom B-Trust-only trust store.
	if len(req.SignedContents) > 0 {
		if err := bisscrpyto.ValidateSignedContents(req.Contents, req.SignedContents, req.SignedContentsCerts); err != nil {
			slog.Warn("signedContents validation failed", "error", err)
			writeError(w, http.StatusUnauthorized, "401", i18n.T("error.signed_contents_failed", err))
			return
		}
	}

	signerCert, err := bisscrpyto.ParseCertBase64(req.SignerCertificateB64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "400", i18n.T("error.invalid_signer_cert", err))
		return
	}

	driver := s.Driver()
	if driver == nil {
		writeError(w, http.StatusServiceUnavailable, "503", i18n.T("pkcs11.no_libraries"))
		return
	}

	allCerts, err := driver.ListCertificates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "500", i18n.T("pkcs11.no_certs_on_token"))
		return
	}

	// Find the CertWithSlot that matches the signerCertificateB64.
	var matchedCert *pkcs11.CertWithSlot
	for _, c := range allCerts {
		if c.Certificate.SerialNumber.Cmp(signerCert.SerialNumber) == 0 &&
			c.Certificate.Issuer.String() == signerCert.Issuer.String() {
			matchedCert = c
			break
		}
	}
	if matchedCert == nil {
		writeError(w, http.StatusNotFound, "404", i18n.T("pkcs11.cert_not_found"))
		return
	}

	confirmHint := ""
	if len(req.ConfirmText) > 0 {
		confirmHint = i18n.T("dialog.pin_confirm", strings.Join(req.ConfirmText, "\n"))
	}
	slog.Info("prompting user for PIN", "cert", matchedCert.Certificate.Subject.CommonName)
	pinBytes, err := s.dialog.PromptPIN(
		i18n.T("dialog.pin_title"),
		i18n.T("dialog.pin_message", matchedCert.Certificate.Subject.CommonName)+confirmHint,
	)
	if err != nil {
		slog.Error("PIN prompt failed", "error", err)
		writeError(w, http.StatusUnauthorized, "401", i18n.T("error.pin_failed", err))
		return
	}
	defer func() {
		for i := range pinBytes {
			pinBytes[i] = 0
		}
	}()

	signer := pkcs11.NewSigner(driver)

	signatures := make([]string, len(req.Contents))
	for i, contentB64 := range req.Contents {
		data, err := base64.StdEncoding.DecodeString(contentB64)
		if err != nil {
			data, err = base64.RawStdEncoding.DecodeString(contentB64)
			if err != nil {
				writeError(w, http.StatusBadRequest, "400", i18n.T("error.base64_decode_failed", i))
				return
			}
		}

		sig, err := signer.Sign(matchedCert, data, pinBytes)
		if err != nil {
			slog.Error("signing failed", "index", i, "error", err)
			writeError(w, http.StatusInternalServerError, "500", i18n.T("error.signing_failed", err))
			return
		}

		signatures[i] = base64.StdEncoding.EncodeToString(sig)
	}

	writeJSON(w, http.StatusOK, signResponse{
		Status:             "ok",
		ReasonCode:         "200",
		Signatures:         signatures,
		SignatureType:      "signature",
		SignatureAlgorithm: "SHA256withRSA",
	})
}

// ---- Helpers ----

// writeJSON serialises v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

// writeError writes a BISS-compatible JSON error response.
func writeError(w http.ResponseWriter, httpStatus int, code, text string) {
	writeJSON(w, httpStatus, errorResponse{
		Status:     "error",
		ReasonCode: code,
		ReasonText: text,
	})
}
