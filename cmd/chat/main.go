// Command chat lets you text the agent from your terminal. No setup needed:
//
//	go run ./cmd/chat
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Trisall7/sms-booking-agent/internal/agent"
	"github.com/Trisall7/sms-booking-agent/internal/booking"
)

func main() {
	cal := booking.NewCalendar(booking.DemoSlots(time.Now(), time.Local))
	ag := agent.New(cal, "Demo Studio", time.Now)

	fmt.Println("Text the booking agent. Try: hi, HELP, STOP, START. Ctrl+D or /quit to exit.")
	in := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("\nyou   > ")
		if !in.Scan() {
			fmt.Println()
			return
		}
		line := in.Text()
		if strings.TrimSpace(line) == "/quit" {
			return
		}
		replies := ag.Handle("+15555550100", line)
		if len(replies) == 0 {
			fmt.Println("agent > (no reply)")
		}
		for _, r := range replies {
			fmt.Println("agent > " + strings.ReplaceAll(r, "\n", "\n        "))
		}
	}
}
