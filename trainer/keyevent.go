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
	// Ctrl) that bypass stdin correlation and have no corresponding stdin byte.
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
// Iambic auto-repeat: holding a paddle emits elements at the configured WPM
// rate, as a hardware keyer would.
//
//   - Press: emit one element immediately, arm the repeat timer.
//   - Repeat timer fires (key still held, other paddle not pressed): emit again
//     and re-arm at the standard WPM period (ditPeriod / dahPeriod).
//   - Other paddle pressed: pause the current paddle's repeat timer to avoid
//     simultaneous timers; resume when that paddle is released.
//   - Release: cancel the repeat timer.
//
// The initial repeat delay (ditFirstRepeat / dahFirstRepeat) is intentionally
// longer than the WPM period so that a single deliberate press-release does not
// accidentally trigger auto-repeat, and to give the operator a moment to decide
// whether to continue holding.  Subsequent repeats use the exact WPM period.
//
// Contact-bounce debounce: press events are rejected if the previous accepted
// event for that key arrived within debounceDuration.  Releases are always
// accepted immediately so that quick simultaneous keying cannot get stuck.
//
// Control keys (Enter, Delete, Quit) emit once on press.
func NewIambicAdapter(keys <-chan KeyEvent, timing Timing) <-chan MorseInput {
	out := make(chan MorseInput, 64)
	go runIambic(keys, timing, out)
	return out
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

func runIambic(keys <-chan KeyEvent, timing Timing, out chan<- MorseInput) {
	defer close(out)

	send := func(k MorseInputKind) { out <- MorseInput{Kind: k} }

	ditPeriod := timing.Dit + timing.ToneGap   // 2×Dit at any WPM
	dahPeriod := timing.Dah + timing.ToneGap   // 4×Dit at any WPM

	// Initial repeat delays before auto-repeat kicks in.
	//
	// Constraints:
	//   (a) Must be > typical single press duration (~100 ms) to avoid
	//       accidental auto-repeat on a normal press.
	//   (b) Must give the operator time to react after hearing the first
	//       element before the second fires.
	//   (c) Must be < charBoundary (6×Dit) so auto-repeat fires before
	//       sendWord flushes the character.
	//
	// Dit (audio = 2×Dit = 120 ms at 20 WPM):
	//   ditFirstRepeat = 3.5×Dit = 210 ms (CharGap + ToneGap/2).
	//
	//   Calibrated from measured press durations:
	//     single tap:          ~50–150 ms  → no repeat needed  (150 < 210 ✓)
	//     intentional hold:    ~226 ms     → repeat fires       (226 > 210 ✓)
	//     longer hold (2-dit): ~303 ms     → 2nd repeat at 330 ms, so released
	//                                         before it → exactly 2 dits       ✓
	//
	// Dah (audio = 4×Dit = 240 ms at 20 WPM):
	//   dahFirstRepeat = 5×Dit = 300 ms → 60 ms margin after audio ends.
	//   Using 4×Dit (= dahPeriod) would leave zero margin.
	ditFirstRepeat := timing.CharGap + timing.ToneGap/2         // 3.5×Dit ≈ 210 ms
	dahFirstRepeat := timing.CharGap + 2*timing.ToneGap         // 5×Dit   = 300 ms

	ditHeld, dahHeld := false, false
	var ditLastEventAt, dahLastEventAt time.Time

	// debounceDuration suppresses contact bounce on USB iambic paddle switches.
	//
	// Mechanical contacts bounce on both press and release:
	//   • Press bounce:   press → bounce-release → bounce-press  (all within ~5 ms)
	//   • Release bounce: release → bounce-press → bounce-release (all within ~5 ms)
	//
	// Only presses are debounced: rejected if the previous accepted dit/dah event
	// (press or release) was < debounceDuration ago.  Releases are never debounced
	// so that quick simultaneous keying (press dit, press dah, release dit all
	// within 15 ms) cannot leave ditHeld stuck true.
	//
	// Measured USB paddle press durations: 30–100 ms.
	// Typical contact bounce duration: < 10 ms.
	// 15 ms filters bounces with a 15 ms margin below the shortest real press.
	const debounceDuration = 15 * time.Millisecond

	ditTimer := time.NewTimer(0)
	drainTimer(ditTimer)
	dahTimer := time.NewTimer(0)
	drainTimer(dahTimer)

	// squeezeTimer fires when both paddles are held simultaneously for
	// squeezeDuration, emitting a Delete to clear the current input without
	// needing to reach for the keyboard.
	squeezeTimer := time.NewTimer(0)
	drainTimer(squeezeTimer)
	const squeezeDuration = 1500 * time.Millisecond // 1.5 s

	handleEvent := func(evt KeyEvent) bool {
		switch evt.Key {
		case KeyDit:
			if evt.Pressed {
				if ditHeld {
					break
				}
				if !ditLastEventAt.IsZero() && evt.At.Sub(ditLastEventAt) < debounceDuration {
					break // bounce re-press
				}
				ditLastEventAt = evt.At
				ditHeld = true
				send(MorseInputDit)
				drainTimer(dahTimer) // pause dah repeat while dit is active
				ditTimer.Reset(ditFirstRepeat)
				if dahHeld {
					squeezeTimer.Reset(squeezeDuration)
				}
			} else if ditHeld {
				ditLastEventAt = evt.At
				ditHeld = false
				drainTimer(ditTimer)
				drainTimer(squeezeTimer)
				if dahHeld {
					dahTimer.Reset(dahPeriod) // resume dah repeat
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
				drainTimer(ditTimer) // pause dit repeat while dah is active
				dahTimer.Reset(dahFirstRepeat)
				if ditHeld {
					squeezeTimer.Reset(squeezeDuration)
				}
			} else if dahHeld {
				dahLastEventAt = evt.At
				dahHeld = false
				drainTimer(dahTimer)
				drainTimer(squeezeTimer)
				if ditHeld {
					ditTimer.Reset(ditPeriod) // resume dit repeat
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

		case <-squeezeTimer.C:
			// Both paddles held for squeezeDuration — emit Delete.
			// Drain element timers so no spurious elements follow.
			drainTimer(ditTimer)
			drainTimer(dahTimer)
			send(MorseInputDelete)
		}
	}
}
