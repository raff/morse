//go:build ignore

package main

import (
	"fmt"
	"log"
	"time"
)

// Run with: make timing
// (see Makefile — excludes main.go to avoid duplicate main)
func main() {
	timing := NewTiming(20, 20)

	stdinChars, restore, err := startRawInput()
	if err != nil {
		log.Fatalf("raw input: %v", err)
	}
	defer restore()

	rawEvents, stop, err := StartSendKeySource(stdinChars, '[', ']')
	if err != nil {
		log.Fatalf("key source: %v", err)
	}
	defer stop()

	fmt.Println("Timing test: press [ (dit) or ] (dah), Ctrl+C to quit.\r")
	fmt.Println("\r")

	// Interpose a logging channel between the key source and IambicAdapter
	// so we can print each raw KeyEvent with its timestamp.
	logged := make(chan KeyEvent, 100)

	go func() {
		var t0 time.Time

		for evt := range rawEvents {
			dir := ""
			if evt.Direct {
				dir = " direct"
			}
			arrow := "↓"
			if evt.Pressed {
				t0 = evt.At
			} else {
				arrow = "↑"
			}

			t := evt.At

			fmt.Printf("%7.1f ms  key=%v %s%s\r\n",
				float64(t.Sub(t0).Microseconds())/1000,
				evt.Key, arrow, dir)
			logged <- evt
		}
		close(logged)
	}()

	morseInputs := NewIambicAdapter(logged, timing)
	t0 := time.Now()

	for inp := range morseInputs {
		t := time.Now()

		fmt.Printf("%7.1f ms  morse=%v\r\n",
			float64(t.Sub(t0).Microseconds())/1000, inp.Kind)
		t0 = t
	}
}

