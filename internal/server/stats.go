package server

import (
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// MaxBodyCapture is the per-direction body size cap (bytes) for the recent-
// requests ring. Bodies larger than this are truncated and the suffix
// "... (N more bytes truncated)" is appended so the UI never balloons memory
// when a client uploads a multi-megabyte /sign payload. 64 KiB comfortably
// fits a typical signedContents + base64 contents pair.
const MaxBodyCapture = 64 * 1024

// RequestRecord is a single captured request in the recent-requests ring.
// In addition to the lightweight counters in EndpointCounters, it stores
// enough of the HTTP envelope (headers + bodies) to render an expandable
// debug view in the API tab. PII risk is low: PIN entry never crosses
// HTTP — it is collected via a native OS dialog. The Authorization header
// is redacted defensively in case future clients add one.
type RequestRecord struct {
	Time            time.Time
	Method          string
	Path            string
	StatusCode      int
	DurationMillis  int64
	RequestHeaders  http.Header
	RequestBody     string
	RequestTrunc    int
	ResponseHeaders http.Header
	ResponseBody    string
	ResponseTrunc   int
}

// EndpointCounters tracks 2xx / 4xx / 5xx + total counts per endpoint.
// Reads via atomic.LoadInt64 stay lock-free under the 2 Hz UI poll.
type EndpointCounters struct {
	Total       atomic.Int64
	Success     atomic.Int64
	ClientError atomic.Int64
	ServerError atomic.Int64
}

// MaxRecentRequests caps the recent-requests ring buffer; oldest entries
// are overwritten when full.
const MaxRecentRequests = 100

// RequestStats aggregates per-endpoint counters + a fixed-capacity ring
// of the most recent requests. Counters are atomic; the ring is mutex-
// protected since reads return a snapshot copy.
type RequestStats struct {
	Version   EndpointCounters
	GetSigner EndpointCounters
	Sign      EndpointCounters
	Other     EndpointCounters

	mu   sync.RWMutex
	ring [MaxRecentRequests]RequestRecord
	head int // next write index
	size int // current count (0..MaxRecentRequests)
}

// RecordRequest writes a request to the appropriate counters and ring.
// Safe for concurrent use from any goroutine; the hot path (counters) is
// lock-free. The caller is responsible for any header redaction; this
// function snapshots the headers it receives.
func (s *RequestStats) RecordRequest(rec RequestRecord) {
	counters := s.bucket(rec.Path)
	counters.Total.Add(1)
	switch {
	case rec.StatusCode >= 500:
		counters.ServerError.Add(1)
	case rec.StatusCode >= 400:
		counters.ClientError.Add(1)
	case rec.StatusCode >= 200 && rec.StatusCode < 300:
		counters.Success.Add(1)
	}

	s.mu.Lock()
	s.ring[s.head] = rec
	s.head = (s.head + 1) % MaxRecentRequests
	if s.size < MaxRecentRequests {
		s.size++
	}
	s.mu.Unlock()
}

// bucket maps a request path to its EndpointCounters. Unknown paths
// (e.g., the /does-not-exist 404 probe) go into Other.
func (s *RequestStats) bucket(path string) *EndpointCounters {
	switch path {
	case "/version":
		return &s.Version
	case "/getsigner":
		return &s.GetSigner
	case "/sign":
		return &s.Sign
	default:
		return &s.Other
	}
}

// Recent returns a snapshot of recent requests in chronological order
// (oldest first). The returned slice is a copy and safe to retain.
func (s *RequestStats) Recent() []RequestRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RequestRecord, s.size)
	start := (s.head - s.size + MaxRecentRequests) % MaxRecentRequests
	for i := 0; i < s.size; i++ {
		out[i] = s.ring[(start+i)%MaxRecentRequests]
	}
	return out
}
