package booking

import (
	"errors"
	"sync"
	"testing"
	"time"
)

var monday = time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

func TestDemoSlotsSkipsWeekends(t *testing.T) {
	friday := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	slots := DemoSlots(friday, time.UTC)
	if len(slots) != 9 {
		t.Fatalf("want 9 slots, got %d", len(slots))
	}
	for _, s := range slots {
		if wd := s.Start.Weekday(); wd == time.Saturday || wd == time.Sunday {
			t.Fatalf("slot on weekend: %v", s.Start)
		}
	}
	if got := slots[0].Start.Weekday(); got != time.Monday {
		t.Fatalf("first slot after Friday should be Monday, got %v", got)
	}
}

func TestAvailableIsSortedAndExcludesBooked(t *testing.T) {
	cal := NewCalendar(DemoSlots(monday, time.UTC))
	first := cal.Available(monday, 0)
	if _, err := cal.Book(first[0].Start, "A", "+1", ""); err != nil {
		t.Fatal(err)
	}
	next := cal.Available(monday, 3)
	if len(next) != 3 {
		t.Fatalf("want 3, got %d", len(next))
	}
	if next[0].Start.Equal(first[0].Start) {
		t.Fatal("booked slot still listed as available")
	}
	for i := 1; i < len(next); i++ {
		if !next[i-1].Start.Before(next[i].Start) {
			t.Fatal("slots not sorted")
		}
	}
}

func TestBookUnknownSlot(t *testing.T) {
	cal := NewCalendar(DemoSlots(monday, time.UTC))
	if _, err := cal.Book(monday, "A", "+1", ""); !errors.Is(err, ErrUnknownSlot) {
		t.Fatalf("want ErrUnknownSlot, got %v", err)
	}
}

// Many customers racing for the same slot: exactly one should win.
func TestBookIsRaceSafe(t *testing.T) {
	cal := NewCalendar(DemoSlots(monday, time.UTC))
	start := cal.Available(monday, 1)[0].Start

	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		wins int
	)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cal.Book(start, "racer", "+1", "")
			if err == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			} else if !errors.Is(err, ErrSlotTaken) {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Fatalf("want exactly 1 winner, got %d", wins)
	}
}
