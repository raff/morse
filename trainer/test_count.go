//go:build ignore

package main

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// Run with: make count
//
// Press/hold/element timing diagnostic with audio feedback.
//
// For each burst of keying, shows a timeline of press↓, element, and release↑
// events with millisecond timestamps from the start of the burst.  This
// reveals exactly when auto-repeat fires relative to how long you held the
// paddle:
//
//	   0ms  ↓ dah
//	   1ms  - (element 1)
//	  92ms  ↑ dah  held 92ms
//	─── got: -  (1 element) ───
//
//	   0ms  ↓ dah
//	   1ms  - (element 1)
//	 240ms  - (element 2)      ← auto-repeat fired here
//	 278ms  ↑ dah  held 278ms
//	─── got: --  (2 elements) ───
func main() {
	timing := NewTiming(20, 20)

	ap, err := NewAudioPlayer(600, 0.5)
	if err != nil {
		log.Fatalf("audio: %v", err)
	}
	defer ap.Close()

	// Short click per element — distinct, non-overlapping, one per event.
	clickTone := append(tone(ap.freq, ap.volume, 60*time.Millisecond),
		silence(20*time.Millisecond)...)

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

	fmt.Print("Press/element timing — [ dit  ] dah  Ctrl+C quit\r\n")
	fmt.Print("Timestamps are ms from the start of each burst.\r\n\r\n")

	// keyLog carries raw press/release events to the main goroutine so they
	// can be interleaved with MorseInput events in the timeline.
	type keyLog struct {
		key     KeyID
		pressed bool
		at      time.Time
	}
	keyLogs := make(chan keyLog, 100)

	// Forward raw events to the IambicAdapter; also send dit/dah press/release
	// to keyLogs so the main goroutine can print them with timestamps.
	logged := make(chan KeyEvent, 100)
	go func() {
		for evt := range rawEvents {
			if evt.Key == KeyDit || evt.Key == KeyDah {
				keyLogs <- keyLog{key: evt.Key, pressed: evt.Pressed, at: evt.At}
			}
			logged <- evt
		}
		close(logged)
		close(keyLogs)
	}()

	inputs := NewIambicAdapter(logged, timing)

	// Flush the accumulated burst after 1.2 s of silence.
	const flushSilence = 1200 * time.Millisecond
	flushTimer := time.NewTimer(0)
	drainTimer(flushTimer)

	var (
		t0          time.Time     // start of current burst; zero between bursts
		lastPress   [2]time.Time  // press time per key (indexed by KeyID)
		pattern     strings.Builder
		nElems      int
	)

	keyName := [2]string{"dit", "dah"}

	// elapsed returns ms since burst start; sets t0 on the first call.
	elapsed := func(t time.Time) int64 {
		if t0.IsZero() {
			t0 = t
		}
		return t.Sub(t0).Milliseconds()
	}

	resetBurst := func() {
		pattern.Reset()
		nElems = 0
		t0 = time.Time{}
		lastPress = [2]time.Time{}
	}

	flush := func() {
		if pattern.Len() == 0 {
			return
		}
		s := pattern.String()
		fmt.Printf("─── got: %-12s (%d %s) ───\r\n\r\n",
			s, nElems, pluralize("element", "elements", nElems))
		resetBurst()
	}

	armFlush := func() {
		drainTimer(flushTimer)
		flushTimer.Reset(flushSilence)
	}

	for {
		select {
		case kl, ok := <-keyLogs:
			if !ok {
				flush()
				return
			}
			t := elapsed(kl.at)
			armFlush()
			if kl.pressed {
				lastPress[kl.key] = kl.at
				fmt.Printf("  %4dms  ↓ %s\r\n", t, keyName[kl.key])
			} else {
				held := ""
				if !lastPress[kl.key].IsZero() {
					held = fmt.Sprintf("  held %dms", kl.at.Sub(lastPress[kl.key]).Milliseconds())
					lastPress[kl.key] = time.Time{}
				}
				fmt.Printf("  %4dms  ↑ %s%s\r\n", t, keyName[kl.key], held)
			}

		case inp, ok := <-inputs:
			if !ok {
				flush()
				return
			}
			now := time.Now()
			switch inp.Kind {
			case MorseInputDit:
				ap.PlayQueued(clickTone)
				t := elapsed(now)
				nElems++
				pattern.WriteByte('.')
				fmt.Printf("  %4dms  . (element %d)\r\n", t, nElems)
				armFlush()

			case MorseInputDah:
				ap.PlayQueued(clickTone)
				t := elapsed(now)
				nElems++
				pattern.WriteByte('-')
				fmt.Printf("  %4dms  - (element %d)\r\n", t, nElems)
				armFlush()

			case MorseInputDelete:
				resetBurst()
				fmt.Print("  [deleted]\r\n")

			case MorseInputQuit:
				flush()
				return
			}

		case <-flushTimer.C:
			flush()
		}
	}
}
