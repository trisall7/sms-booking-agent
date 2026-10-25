// Package booking holds a small, concurrency-safe appointment calendar.
//
// It is in-memory on purpose so the demo runs with zero setup. In production
// the same guarantees (no double-booking) come from a unique constraint or a
// row lock in the database; the Book method mirrors that contract.
package booking

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

var (
	// ErrSlotTaken means someone else booked the slot first.
	ErrSlotTaken = errors.New("slot is no longer available")
	// ErrUnknownSlot means the requested start time is not on the calendar.
	ErrUnknownSlot = errors.New("slot does not exist")
)

// Slot is one bookable time window.
type Slot struct {
	Start    time.Time
	Duration time.Duration
}

// Booking is a confirmed appointment.
type Booking struct {
	ID    string
	Slot  Slot
	Name  string
	Phone string
	Notes string
}

// Calendar tracks open and booked slots.
type Calendar struct {
	mu     sync.Mutex
	slots  map[int64]Slot
	booked map[int64]Booking
	seq    int
}

// NewCalendar returns a calendar containing the given open slots.
func NewCalendar(slots []Slot) *Calendar {
	c := &Calendar{
		slots:  make(map[int64]Slot, len(slots)),
		booked: make(map[int64]Booking),
	}
	for _, s := range slots {
		c.slots[s.Start.Unix()] = s
	}
	return c
}

// DemoSlots builds sample availability: the next three weekdays after now,
// at 11:00, 14:00 and 17:00 in loc, two hours each.
func DemoSlots(now time.Time, loc *time.Location) []Slot {
	now = now.In(loc)
	var out []Slot
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	for days := 0; days < 3; {
		day = day.AddDate(0, 0, 1)
		if wd := day.Weekday(); wd == time.Saturday || wd == time.Sunday {
			continue
		}
		for _, h := range []int{11, 14, 17} {
			out = append(out, Slot{
				Start:    day.Add(time.Duration(h) * time.Hour),
				Duration: 2 * time.Hour,
			})
		}
		days++
	}
	return out
}

// Available returns up to limit open slots starting after the given time,
// earliest first.
func (c *Calendar) Available(after time.Time, limit int) []Slot {
	c.mu.Lock()
	defer c.mu.Unlock()

	var out []Slot
	for key, s := range c.slots {
		if _, taken := c.booked[key]; taken || !s.Start.After(after) {
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start.Before(out[j].Start) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Book reserves the slot starting at start. It is safe to call concurrently:
// exactly one caller wins a given slot and the rest get ErrSlotTaken.
func (c *Calendar) Book(start time.Time, name, phone, notes string) (Booking, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := start.Unix()
	slot, ok := c.slots[key]
	if !ok {
		return Booking{}, ErrUnknownSlot
	}
	if _, taken := c.booked[key]; taken {
		return Booking{}, ErrSlotTaken
	}
	c.seq++
	b := Booking{
		ID:    fmt.Sprintf("BK-%04d", c.seq),
		Slot:  slot,
		Name:  name,
		Phone: phone,
		Notes: notes,
	}
	c.booked[key] = b
	return b, nil
}
