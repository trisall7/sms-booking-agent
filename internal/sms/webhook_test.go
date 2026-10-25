package sms

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

var testSecret = []byte("test-secret-not-used-anywhere-real")

type echoResponder struct{}

func (echoResponder) Handle(from, body string) []string { return []string{"echo: " + body} }

type recordingSender struct {
	mu   sync.Mutex
	sent []string
}

func (s *recordingSender) Send(_ context.Context, _, _, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, text)
	return nil
}

func newTestHandler(now time.Time) (*Handler, *recordingSender) {
	rs := &recordingSender{}
	h := NewHandler(testSecret, echoResponder{}, rs)
	h.now = func() time.Time { return now }
	return h, rs
}

func post(h http.Handler, body []byte, ts time.Time, secret []byte) int {
	stamp := strconv.FormatInt(ts.Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/sms", bytes.NewReader(body))
	req.Header.Set(HeaderTimestamp, stamp)
	req.Header.Set(HeaderSignature, Sign(secret, stamp, body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

var payload = []byte(`{"id":"msg-1","from":"+15555550100","to":"+15555550199","text":"hi"}`)

func TestValidSignatureIsProcessed(t *testing.T) {
	now := time.Now()
	h, rs := newTestHandler(now)
	if code := post(h, payload, now, testSecret); code != http.StatusOK {
		t.Fatalf("want 200, got %d", code)
	}
	if len(rs.sent) != 1 || rs.sent[0] != "echo: hi" {
		t.Fatalf("unexpected sends: %v", rs.sent)
	}
}

func TestWrongSecretRejected(t *testing.T) {
	now := time.Now()
	h, rs := newTestHandler(now)
	if code := post(h, payload, now, []byte("wrong")); code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", code)
	}
	if len(rs.sent) != 0 {
		t.Fatal("rejected request still sent a reply")
	}
}

func TestStaleTimestampRejected(t *testing.T) {
	now := time.Now()
	h, _ := newTestHandler(now)
	if code := post(h, payload, now.Add(-10*time.Minute), testSecret); code != http.StatusUnauthorized {
		t.Fatalf("want 401 for replayed request, got %d", code)
	}
}

func TestTamperedBodyRejected(t *testing.T) {
	now := time.Now()
	h, _ := newTestHandler(now)
	stamp := strconv.FormatInt(now.Unix(), 10)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte(`{"id":"x","from":"+1","text":"evil"}`)))
	req.Header.Set(HeaderTimestamp, stamp)
	req.Header.Set(HeaderSignature, Sign(testSecret, stamp, payload))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestDuplicateDeliveryProcessedOnce(t *testing.T) {
	now := time.Now()
	h, rs := newTestHandler(now)
	for i := 0; i < 3; i++ {
		if code := post(h, payload, now, testSecret); code != http.StatusOK {
			t.Fatalf("attempt %d: want 200, got %d", i, code)
		}
	}
	if len(rs.sent) != 1 {
		t.Fatalf("want 1 reply for 3 deliveries, got %d", len(rs.sent))
	}
}

func TestOnlyPostAllowed(t *testing.T) {
	h, _ := newTestHandler(time.Now())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}
