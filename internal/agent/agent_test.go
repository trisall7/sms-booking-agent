package agent

import (
	"strings"
	"testing"
	"time"

	"github.com/trisall7/sms-booking-agent/internal/booking"
)

var monday = time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

func newTestAgent() *Agent {
	cal := booking.NewCalendar(booking.DemoSlots(monday, time.UTC))
	return New(cal, "Demo Studio", func() time.Time { return monday })
}

// say sends text and asserts the single reply contains want.
func say(t *testing.T, a *Agent, from, text, want string) string {
	t.Helper()
	replies := a.Handle(from, text)
	if len(replies) != 1 {
		t.Fatalf("%q: want 1 reply, got %d: %v", text, len(replies), replies)
	}
	if !strings.Contains(replies[0], want) {
		t.Fatalf("%q: reply %q does not contain %q", text, replies[0], want)
	}
	return replies[0]
}

func TestHappyPathBooking(t *testing.T) {
	a := newTestAgent()
	const from = "+15555550100"

	say(t, a, from, "hi", "What's your name?")
	say(t, a, from, "Sam", "Nice to meet you, Sam")
	say(t, a, from, "A small consultation", "Reply with 1-3")
	say(t, a, from, "1", "Tue Oct 27 at 11:00 AM")
	say(t, a, from, "yes", "Confirmation BK-0001")
	say(t, a, from, "thanks!", "Text NEW")
}

func TestInvalidSlotChoice(t *testing.T) {
	a := newTestAgent()
	const from = "+15555550101"
	for _, msg := range []string{"hi", "Ana", "touch-up"} {
		a.Handle(from, msg)
	}
	say(t, a, from, "9", "number from 1-3")
	say(t, a, from, "two", "number from 1-3")
	say(t, a, from, "2", "Reply YES")
}

func TestSlotTakenWhileConfirming(t *testing.T) {
	a := newTestAgent()
	for _, from := range []string{"+1A", "+1B"} {
		for _, msg := range []string{"hi", "Name", "request", "1"} {
			a.Handle(from, msg)
		}
	}
	say(t, a, "+1A", "YES", "Confirmation BK-0001")
	reply := say(t, a, "+1B", "YES", "just taken")
	if strings.Contains(reply, "Tue Oct 27 at 11:00 AM") {
		t.Fatal("taken slot offered again")
	}
}

func TestStopAndStart(t *testing.T) {
	a := newTestAgent()
	const from = "+15555550102"

	say(t, a, from, "hi", "What's your name?")
	say(t, a, from, "STOP", "unsubscribed")
	if r := a.Handle(from, "hello?"); r != nil {
		t.Fatalf("opted-out user got a reply: %v", r)
	}
	if r := a.Handle(from, "stop"); r != nil {
		t.Fatalf("repeat STOP should be silent, got %v", r)
	}
	say(t, a, from, "start", "resubscribed")
	say(t, a, from, "Lee", "Nice to meet you, Lee")
}

func TestHelpDoesNotChangeState(t *testing.T) {
	a := newTestAgent()
	const from = "+15555550103"
	say(t, a, from, "hi", "What's your name?")
	say(t, a, from, "help", "Reply STOP")
	say(t, a, from, "Jo", "Nice to meet you, Jo")
}
