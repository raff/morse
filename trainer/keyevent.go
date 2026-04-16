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
	//   (a) Must be > typical key-press hold time (~100 ms) so a single
	//       deliberate press does not accidentally trigger a repeat.
	//   (b) Must be < charBoundary (2×CharGap = 6×Dit) so that intentional
	//       hold-to-repeat fires before sendWord flushes the character.
	//
	// CharGap + ToneGap = 4×Dit satisfies both at any WPM:
	//   20 WPM → 240 ms  (charBoundary = 360 ms, margin = 140 ms)
	//   30 WPM → 160 ms  (charBoundary = 240 ms, margin = 60 ms)
	// Subsequent repeats use the normal ditPeriod / dahPeriod.
	//
	// TODO: 4×Dit is still noticeably longer than the 2×Dit repeat rate,
	// producing an uneven feel (long pause then fast repeats). Ideally
	// firstRepeat ≈ ditPeriod, but the ~100ms paddle press duration leaves
	// insufficient margin at typical WPM rates. Options: hardware debounce
	// data, adaptive threshold based on press duration, or a small constant
	// added to ditPeriod with a measured safety floor.
	firstRepeat := timing.CharGap + timing.ToneGap

	ditHeld, dahHeld := false, false
	var ditLastEventAt, dahLastEventAt time.Time

	// debounceDuration suppresses contact bounce on USB iambic paddle switches.
	//
	// Mechanical contacts bounce on both press and release:
	//   • Press bounce:   press → bounce-release → bounce-press  (all within ~5 ms)
	//   • Release bounce: release → bounce-press → bounce-release (all within ~5 ms)
	//
	// We use a single "last event" timestamp per key: ignore any event (press or
	// release) that arrives within debounceDuration of the previous event for that
	// key.  This blocks all bounce events regardless of direction.
	//
	// Measured USB paddle press durations: 30–100 ms.
	// Typical contact bounce duration: < 10 ms.
	// 15 ms filters bounces with a 15 ms margin below the shortest real press.
	const debounceDuration = 15 * time.Millisecond

	ditTimer := time.NewTimer(0)
	drainTimer(ditTimer)
	dahTimer := time.NewTimer(0)
	drainTimer(dahTimer)

	// handleEvent processes a single KeyEvent. Returns true if the goroutine
	// should exit.
	handleEvent := func(evt KeyEvent) bool {
		switch evt.Key {
		case KeyDit:
			if evt.Pressed {
				if ditHeld {
					break // already held
				}
				// Debounce presses only: reject if last accepted dit event
				// (press or release) was too recent — that's a bounce re-press.
				// Releases are never debounced so quick simultaneous keying
				// (press dit, press dah, release dit within 15 ms) still works.
				if !ditLastEventAt.IsZero() && evt.At.Sub(ditLastEventAt) < debounceDuration {
					break
				}
				ditLastEventAt = evt.At
				ditHeld = true
				send(MorseInputDit)
				// Pressing dit pauses dah auto-repeat to avoid spurious elements
				// when the paddles overlap slightly.
				drainTimer(dahTimer)
				ditTimer.Reset(firstRepeat)
			} else if ditHeld {
				ditLastEventAt = evt.At
				ditHeld = false
				drainTimer(ditTimer)
				// Resume dah auto-repeat if dah paddle is still held.
				if dahHeld {
					dahTimer.Reset(dahPeriod)
				}
			}
		case KeyDah:
			if evt.Pressed {
				if dahHeld {
					break
				}
				if !dahLastEventAt.IsZero() && evt.At.Sub(dahLastEventAt) < debounceDuration {
					break
				}
				dahLastEventAt = evt.At
				dahHeld = true
				send(MorseInputDah)
				// Pressing dah pauses dit auto-repeat.
				drainTimer(ditTimer)
				dahTimer.Reset(firstRepeat)
			} else if dahHeld {
				dahLastEventAt = evt.At
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
