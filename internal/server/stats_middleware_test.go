package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStatsMiddlewareCapturesRequestAndResponseEnvelope(t *testing.T) {
	srv := &Server{}

	handler := srv.statsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, 0, 64)
		buf := make([]byte, 1024)
		for {
			n, err := r.Body.Read(buf)
			body = append(body, buf[:n]...)
			if err != nil {
				break
			}
		}
		if string(body) != `{"selector":{"x":1}}` {
			t.Errorf("handler saw body %q, want intact restored body", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","reasonCode":"200"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/getsigner", strings.NewReader(`{"selector":{"x":1}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Test", "qa")
	req.Header.Set("Authorization", "Bearer SECRET")

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	records := srv.Stats().Recent()
	if len(records) != 1 {
		t.Fatalf("Recent() len = %d, want 1", len(records))
	}
	rec := records[0]

	if rec.Method != "POST" || rec.Path != "/getsigner" || rec.StatusCode != 200 {
		t.Errorf("bad summary: method=%s path=%s status=%d", rec.Method, rec.Path, rec.StatusCode)
	}
	if rec.RequestBody != `{"selector":{"x":1}}` {
		t.Errorf("RequestBody = %q, want intact JSON", rec.RequestBody)
	}
	if rec.RequestTrunc != 0 {
		t.Errorf("RequestTrunc = %d, want 0", rec.RequestTrunc)
	}
	if !strings.Contains(rec.ResponseBody, `"status":"ok"`) {
		t.Errorf("ResponseBody missing payload: %q", rec.ResponseBody)
	}
	if rec.RequestHeaders.Get("X-Test") != "qa" {
		t.Errorf("X-Test header not captured")
	}
	if got := rec.RequestHeaders.Get("Authorization"); got != "***" {
		t.Errorf("Authorization not redacted: %q", got)
	}
	if rec.ResponseHeaders.Get("Content-Type") != "application/json" {
		t.Errorf("response Content-Type not captured")
	}
}

func TestStatsMiddlewareTruncatesOversizedRequestBody(t *testing.T) {
	srv := &Server{}

	handler := srv.statsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := 0
		buf := make([]byte, 1024)
		for {
			n, err := r.Body.Read(buf)
			count += n
			if err != nil {
				break
			}
		}
		if count != MaxBodyCapture+8 {
			t.Errorf("handler received %d bytes, want full %d", count, MaxBodyCapture+8)
		}
		w.WriteHeader(http.StatusOK)
	}))

	payload := strings.Repeat("a", MaxBodyCapture+8)
	req := httptest.NewRequest(http.MethodPost, "/sign", strings.NewReader(payload))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	rec := srv.Stats().Recent()[0]
	if len(rec.RequestBody) != MaxBodyCapture {
		t.Errorf("captured len = %d, want %d", len(rec.RequestBody), MaxBodyCapture)
	}
	if rec.RequestTrunc != 8 {
		t.Errorf("RequestTrunc = %d, want 8", rec.RequestTrunc)
	}
}

func TestStatsMiddlewareSkipsDocsPaths(t *testing.T) {
	srv := &Server{}

	handler := srv.statsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>swagger ui assets...</html>"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/docs/index.html", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	rec := srv.Stats().Recent()[0]
	if rec.RequestBody != "" || rec.ResponseBody != "" {
		t.Errorf("bodies should be empty for /docs paths: req=%q resp=%q", rec.RequestBody, rec.ResponseBody)
	}
	if rec.StatusCode != 200 {
		t.Errorf("status not captured for /docs path: %d", rec.StatusCode)
	}
}
