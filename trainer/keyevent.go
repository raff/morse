package main

import "time"

// KeyID identifies a logical key action.
type KeyID int

const (
	KeyDit    KeyID = iota // dit paddle: [ or left-Ctrl
	KeyDah                 // dah paddle: ] or right-Ctrl
	KeyEnter               // submit word
	KeyDelete              // clear and retry
	KeyQuit                // Ctrl+C / Ctrl+D / EOF
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
	MorseInputDit MorseInputKind = iota
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
//   - Press: emit one element immediately, arm the repeat timer at ditPeriod /
//     dahPeriod (the exact WPM period, same as all subsequent repeats).
//   - Repeat timer fires (key still held, other paddle not pressed): emit again
//     and re-arm.
//   - Other paddle pressed: pause the current paddle's repeat timer to avoid
//     simultaneous timers; resume when that paddle is released.
//   - Release: cancel the repeat timer.
//
// Idle-delete: if no paddle is pressed for idleDuration (1.5 s), a
// MorseInputDelete is emitted to clear the current input without reaching for
// the keyboard.
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

	ditPeriod := timing.Dit + timing.ToneGap // 2×Dit at any WPM
	dahPeriod := timing.Dah + timing.ToneGap // 4×Dit at any WPM

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

	// idleTimer fires when no paddle has been pressed for idleDuration, emitting
	// a Delete to clear the current input without needing to reach for the
	// keyboard. It is armed when the last held paddle is released and canceled
	// when either paddle is pressed.
	idleTimer := time.NewTimer(0)
	drainTimer(idleTimer)
	const idleDuration = 1500 * time.Millisecond

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
				drainTimer(idleTimer)
				send(MorseInputDit)
				drainTimer(dahTimer) // pause dah repeat while dit is active
				ditTimer.Reset(ditPeriod)
			} else if ditHeld {
				held := evt.At.Sub(ditLastEventAt)
				ditLastEventAt = evt.At
				ditHeld = false
				drainTimer(ditTimer)
				if dahHeld {
					// Dah still held: resume dah repeat (unless this was a bounce).
					if held >= debounceDuration {
						dahTimer.Reset(dahPeriod)
					}
				} else {
					// Both paddles now up: start idle countdown.
					idleTimer.Reset(idleDuration)
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
				drainTimer(idleTimer)
				send(MorseInputDah)
				drainTimer(ditTimer) // pause dit repeat while dah is active
				dahTimer.Reset(dahPeriod)
			} else if dahHeld {
				held := evt.At.Sub(dahLastEventAt)
				dahLastEventAt = evt.At
				dahHeld = false
				drainTimer(dahTimer)
				if ditHeld {
					// Dit still held: resume dit repeat (unless this was a bounce).
					if held >= debounceDuration {
						ditTimer.Reset(ditPeriod)
					}
				} else {
					// Both paddles now up: start idle countdown.
					idleTimer.Reset(idleDuration)
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

		case <-idleTimer.C:
			// No paddle pressed for idleDuration — clear the current input.
			send(MorseInputDelete)
		}
	}
}
