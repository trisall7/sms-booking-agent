// Package sms receives inbound text messages over a signed webhook and sends
// replies through a pluggable Sender.
//
// The signing scheme is deliberately generic (HMAC-SHA256 over
// "timestamp.body") so the demo doesn't depend on any one carrier. Real
// providers use the same ideas: a shared or public-key signature, a timestamp
// to block replays, and message IDs for de-duplication.
package sms

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	HeaderSignature = "X-Webhook-Signature"
	HeaderTimestamp = "X-Webhook-Timestamp"

	maxBodyBytes   = 64 << 10
	defaultMaxSkew = 5 * time.Minute
	dedupeTTL      = 24 * time.Hour
)

// InboundMessage is the webhook payload.
type InboundMessage struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text"`
}

// Responder turns an inbound message into replies.
type Responder interface {
	Handle(from, body string) []string
}

// Sender delivers an outbound message.
type Sender interface {
	Send(ctx context.Context, to, from, text string) error
}

// LogSender prints outbound messages instead of sending them.
type LogSender struct{}

func (LogSender) Send(_ context.Context, to, from, text string) error {
	log.Printf("SEND %s -> %s: %q", from, to, text)
	return nil
}

// Sign returns the hex signature for a timestamp and body.
func Sign(secret []byte, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// Handler is the HTTP endpoint for inbound messages.
type Handler struct {
	secret    []byte
	responder Responder
	sender    Sender
	now       func() time.Time
	maxSkew   time.Duration

	mu   sync.Mutex
	seen map[string]time.Time
}

// NewHandler builds a webhook handler. secret must not be empty.
func NewHandler(secret []byte, r Responder, s Sender) *Handler {
	return &Handler{
		secret:    secret,
		responder: r,
		sender:    s,
		now:       time.Now,
		maxSkew:   defaultMaxSkew,
		seen:      make(map[string]time.Time),
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}

	if !h.verify(r.Header.Get(HeaderTimestamp), r.Header.Get(HeaderSignature), body) {
		// Same response for every failure so callers learn nothing.
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var msg InboundMessage
	if err := json.Unmarshal(body, &msg); err != nil || msg.ID == "" || msg.From == "" {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}

	// Providers retry on timeouts; never process the same message twice.
	if h.alreadySeen(msg.ID) {
		w.WriteHeader(http.StatusOK)
		return
	}

	for _, reply := range h.responder.Handle(msg.From, msg.Text) {
		if err := h.sender.Send(r.Context(), msg.From, msg.To, reply); err != nil {
			// The conversation has already advanced, so we log rather than
			// ask the provider to retry (which would replay the step).
			log.Printf("send to %s failed: %v", msg.From, err)
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) verify(ts, sig string, body []byte) bool {
	if len(h.secret) == 0 || ts == "" || sig == "" {
		return false
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if d := h.now().Sub(time.Unix(unix, 0)); d > h.maxSkew || d < -h.maxSkew {
		return false
	}
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	want, _ := hex.DecodeString(Sign(h.secret, ts, body))
	return hmac.Equal(got, want)
}

func (h *Handler) alreadySeen(id string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	if t, ok := h.seen[id]; ok && now.Sub(t) < dedupeTTL {
		return true
	}
	h.seen[id] = now
	if len(h.seen) > 10000 {
		for k, t := range h.seen {
			if now.Sub(t) >= dedupeTTL {
				delete(h.seen, k)
			}
		}
	}
	return false
}
