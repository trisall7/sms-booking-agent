// Package agent is the conversation engine: it turns inbound text messages
// into replies and books appointments along the way.
//
// It is a deterministic state machine so the demo runs offline and is fully
// testable. In a production system an LLM can sit behind the same Handle
// method, using the calendar as its tools (check availability, create
// booking) while this kind of state tracking keeps it on rails.
package agent

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Trisall7/sms-booking-agent/internal/booking"
)

type state int

const (
	stateNew state = iota
	stateAskName
	stateAskRequest
	stateChooseSlot
	stateConfirm
	stateBooked
)

const slotsToOffer = 3

type session struct {
	state    state
	optedOut bool
	name     string
	request  string
	offered  []booking.Slot
	chosen   booking.Slot
	booking  booking.Booking
}

// Agent holds one conversation per phone number.
type Agent struct {
	mu       sync.Mutex
	cal      *booking.Calendar
	business string
	now      func() time.Time
	sessions map[string]*session
}

// New creates an agent that books into cal on behalf of business.
func New(cal *booking.Calendar, business string, now func() time.Time) *Agent {
	if now == nil {
		now = time.Now
	}
	return &Agent{
		cal:      cal,
		business: business,
		now:      now,
		sessions: make(map[string]*session),
	}
}

// Handle processes one inbound message and returns the replies to send.
// A nil result means "send nothing" (for example, after the sender opted out).
func (a *Agent) Handle(from, body string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	s, ok := a.sessions[from]
	if !ok {
		s = &session{}
		a.sessions[from] = s
	}
	text := strings.TrimSpace(body)
	keyword := strings.ToUpper(text)

	// Carrier-required keywords are handled before anything else.
	switch keyword {
	case "STOP", "STOPALL", "UNSUBSCRIBE", "CANCEL", "END", "QUIT":
		if s.optedOut {
			return nil
		}
		s.optedOut = true
		return []string{fmt.Sprintf("You're unsubscribed from %s and won't get more messages. Reply START to resubscribe.", a.business)}
	case "START", "UNSTOP":
		s.optedOut = false
		return []string{fmt.Sprintf("You're resubscribed to %s. Text anything to book.", a.business)}
	case "HELP", "INFO":
		return []string{fmt.Sprintf("%s booking assistant. Text to book a consultation. Reply STOP to opt out. Msg & data rates may apply.", a.business)}
	}
	if s.optedOut {
		return nil
	}

	switch s.state {
	case stateNew:
		s.state = stateAskName
		return []string{fmt.Sprintf("Hi! Thanks for texting %s. I can book you a consultation. What's your name?", a.business)}

	case stateAskName:
		name := truncate(text, 40)
		if name == "" {
			return []string{"Sorry, I didn't catch that. What's your name?"}
		}
		s.name = name
		s.state = stateAskRequest
		return []string{fmt.Sprintf("Nice to meet you, %s! In a sentence or two, what would you like to get done?", name)}

	case stateAskRequest:
		if text == "" {
			return []string{"Just a short description is fine. What would you like to get done?"}
		}
		s.request = truncate(text, 280)
		return []string{a.offerSlots(s, "")}

	case stateChooseSlot:
		n, err := strconv.Atoi(text)
		if err != nil || n < 1 || n > len(s.offered) {
			return []string{fmt.Sprintf("Please reply with a number from 1-%d.", len(s.offered))}
		}
		s.chosen = s.offered[n-1]
		s.state = stateConfirm
		return []string{fmt.Sprintf("Book %s? Reply YES to confirm or NO to see other times.", formatSlot(s.chosen))}

	case stateConfirm:
		switch keyword {
		case "YES", "Y", "CONFIRM":
			b, err := a.cal.Book(s.chosen.Start, s.name, from, s.request)
			if errors.Is(err, booking.ErrSlotTaken) {
				return []string{a.offerSlots(s, "Sorry, that time was just taken. ")}
			}
			if err != nil {
				return []string{"Something went wrong saving your booking. Please try again in a moment."}
			}
			s.booking = b
			s.state = stateBooked
			return []string{fmt.Sprintf("You're booked for %s. Confirmation %s. See you then, %s!", formatSlot(b.Slot), b.ID, s.name)}
		case "NO", "N":
			return []string{a.offerSlots(s, "No problem. ")}
		default:
			return []string{"Please reply YES to confirm or NO to see other times."}
		}

	case stateBooked:
		if keyword == "NEW" {
			s.state = stateAskRequest
			return []string{"Sure! What would you like to get done this time?"}
		}
		return []string{fmt.Sprintf("You're booked for %s (confirmation %s). Text NEW to make another booking.", formatSlot(s.booking.Slot), s.booking.ID)}
	}
	return nil
}

// offerSlots lists fresh availability and moves the session to slot choice.
func (a *Agent) offerSlots(s *session, prefix string) string {
	s.offered = a.cal.Available(a.now(), slotsToOffer)
	if len(s.offered) == 0 {
		s.state = stateAskRequest
		return prefix + "We're fully booked right now. Text back later and I'll check again."
	}
	s.state = stateChooseSlot
	var b strings.Builder
	b.WriteString(prefix)
	b.WriteString("Here are the next open times:\n")
	for i, slot := range s.offered {
		fmt.Fprintf(&b, "%d) %s\n", i+1, formatSlot(slot))
	}
	fmt.Fprintf(&b, "Reply with 1-%d.", len(s.offered))
	return b.String()
}

func formatSlot(s booking.Slot) string {
	return s.Start.Format("Mon Jan 2 at 3:04 PM")
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max])
	}
	return s
}
