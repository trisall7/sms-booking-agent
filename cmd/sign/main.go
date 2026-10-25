// Command sign prints curl-ready headers for a payload so you can call the
// running server by hand:
//
//	go run ./cmd/sign testdata/inbound.json
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/Trisall7/sms-booking-agent/internal/sms"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: sign <payload.json>")
	}
	secret := os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		log.Fatal("WEBHOOK_SECRET is not set")
	}
	body, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	fmt.Printf("-H '%s: %s' -H '%s: %s'\n",
		sms.HeaderTimestamp, ts,
		sms.HeaderSignature, sms.Sign([]byte(secret), ts, body))
}
