package main

import "time"

// KeyID identifies a logical key action.
type KeyID int

const (
	KeyDit   KeyID = iota // dit paddle: [ or left-Ctrl
	KeyDah                 // dah paddle: ] or right-Ctrl
	KeyEnter              // submit word
	KeyDelete             // clear and retry
	KeyQuit               // Ctrl+C / Ctrl+D / EOF
)

// KeyEvent is a raw key press or release event with a timestamp.
type KeyEvent struct {
	Key     KeyID
	Pressed bool // true = pressed, false = released
	At      time.Time
	// Direct is true for modifier-key events (USB iambic paddle via left/right
	// Ctrl) that bypass stdin correlation.  The IambicAdapter uses a shorter
	// initial repeat delay for direct events so that paddle hold-to-repeat
	// fires at the configured WPM rate rather than the slower keyboard window.
	Direct bool
}

// MorseInputKind is the event type consumed by sendWord.
type MorseInputKind int

const (
	MorseInputDit    MorseInputKind = iota
	MorseInputDah
	MorseInputDelete
	MorseInputSubmit
	MorseInputQuit
)

// MorseInput is a single processed event for the Morse word-sender.
type MorseInput struct {
	Kind MorseInputKind
}

// NewIambicAdapter wraps a KeyEvent channel and produces a MorseInput channel.
//
// Dit/dah keys auto-repeat at the configured WPM rate while held:
//   - Key press: emit one element immediately, arm a repeat timer.
//   - Repeat timer fires (key still held, other paddle not held): emit again.
//   - Other paddle pressed: stop the current paddle's auto-repeat to prevent
//     simultaneous timers generating spurious extra elements (e.g. 'r' instead
//     of 'a').  When the other paddle is released the paused timer is restarted.
//   - Key release: cancel the repeat timer.
//
// Control keys (Enter, Delete, Quit) emit once on press.
func NewIambicAdapter(keys <-chan KeyEvent, timing Timing) <-chan MorseInput {
	out := make(chan MorseInput, 64)
	go runIambic(keys, timing, out)
	return out
}

func runIambic(keys <-chan KeyEvent, timing Timing, out chan<- MorseInput) {
	defer close(out)

	send := func(k MorseInputKind) { out <- MorseInput{Kind: k} }

	ditPeriod := timing.Dit + timing.ToneGap
	dahPeriod := timing.Dah + timing.ToneGap

	// Initial repeat delay — fires before the first auto-repeat.
	//
	// Two constraints:
	//   (a) Must be > typical key-press hold time (~150 ms) so a single
	//       deliberate press does not accidentally trigger a repeat.
	//   (b) Must be < charBoundary (2×CharGap = 6×Dit) so that intentional
	//       hold-to-repeat fires before sendWord flushes the character.
	//
	// CharGap + 2×ToneGap = 5×Dit satisfies both at any WPM:
	//   20 WPM → 300 ms  (charBoundary = 360 ms)
	//   30 WPM → 200 ms  (charBoundary = 240 ms)
	// Subsequent repeats use the normal ditPeriod / dahPeriod.
	firstRepeat := timing.CharGap + 2*timing.ToneGap

	ditHeld, dahHeld := false, false

	ditTimer := time.NewTimer(0)
	drainTimer(ditTimer)
	dahTimer := time.NewTimer(0)
	drainTimer(dahTimer)

	// handleEvent processes a single KeyEvent. Returns true if the goroutine
	// should exit.
	handleEvent := func(evt KeyEvent) bool {
		switch evt.Key {
		case KeyDit:
			if evt.Pressed && !ditHeld {
				ditHeld = true
				send(MorseInputDit)
				// Pressing dit pauses dah auto-repeat to avoid spurious elements
				// when the paddles overlap slightly.
				drainTimer(dahTimer)
				ditTimer.Reset(firstRepeat)
			} else if !evt.Pressed && ditHeld {
				ditHeld = false
				drainTimer(ditTimer)
				// Resume dah auto-repeat if dah paddle is still held.
				if dahHeld {
					dahTimer.Reset(dahPeriod)
				}
			}
		case KeyDah:
			if evt.Pressed && !dahHeld {
				dahHeld = true
				send(MorseInputDah)
				// Pressing dah pauses dit auto-repeat.
				drainTimer(ditTimer)
				dahTimer.Reset(firstRepeat)
			} else if !evt.Pressed && dahHeld {
				dahHeld = false
				drainTimer(dahTimer)
				// Resume dit auto-repeat if dit paddle is still held.
				if ditHeld {
					ditTimer.Reset(ditPeriod)
				}
			}
		case KeyEnter:
			if evt.Pressed {
				send(MorseInputSubmit)
			}
		case KeyDelete:
			if evt.Pressed {
				send(MorseInputDelete)
			}
		case KeyQuit:
			if evt.Pressed {
				send(MorseInputQuit)
			}
			return true
		}
		return false
	}

	for {
		select {
		case evt, ok := <-keys:
			if !ok {
				return
			}
			if handleEvent(evt) {
				return
			}

		case <-ditTimer.C:
			// Priority-peek: if a key event arrived at the same time as the
			// timer tick, process it first to avoid spurious extra elements.
			select {
			case evt, ok := <-keys:
				if !ok {
					return
				}
				if handleEvent(evt) {
					return
				}
			default:
			}
			if ditHeld && !dahHeld {
				send(MorseInputDit)
				ditTimer.Reset(ditPeriod)
			}

		case <-dahTimer.C:
			// Priority-peek (same rationale as dit timer).
			select {
			case evt, ok := <-keys:
				if !ok {
					return
				}
				if handleEvent(evt) {
					return
				}
			default:
			}
			if dahHeld && !ditHeld {
				send(MorseInputDah)
				dahTimer.Reset(dahPeriod)
			}
		}
	}
}

// drainTimer stops t and drains any pending tick, leaving it in a clean state.
func drainTimer(t *time.Timer) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
}
