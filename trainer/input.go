package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

const stdinFd = 0

// startRawInput puts the terminal in raw mode and launches a goroutine that
// delivers individual bytes from stdin to the returned channel as they arrive.
// Raw mode disables line-buffering and echo; the caller must echo characters
// explicitly. term.MakeRaw also disables OPOST, so all output in this mode
// must use \r\n instead of \n.
// The returned restore function must be called before the program exits.
func startRawInput() (<-chan byte, func(), error) {
	if !term.IsTerminal(stdinFd) {
		return nil, nil, fmt.Errorf("-check requires an interactive terminal")
	}
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		return nil, nil, err
	}
	restore := func() { term.Restore(stdinFd, oldState) }

	ch := make(chan byte, 32)
	go func() {
		buf := make([]byte, 1)
		for {
			n, err := os.Stdin.Read(buf)
			if n > 0 {
				ch <- buf[0]
			}
			if err != nil {
				close(ch)
				return
			}
		}
	}()
	return ch, restore, nil
}

// askUser handles one quiz entry in raw terminal mode.
// Characters arrive one byte at a time so the timeout fires as soon as it
// expires, regardless of whether the user has pressed Enter.
// An empty submission replays the audio and resets the timeout.
// Returns (correct, quit).
func askUser(chars <-chan byte, ap *AudioPlayer, audio []byte, expected string, n int, timeout time.Duration) (correct, quit bool) {
	for {
		fmt.Printf("[%d] > ", n)
		var input []byte

		// A nil channel blocks forever, effectively disabling the timeout.
		var timer <-chan time.Time
		if timeout > 0 {
			timer = time.After(timeout)
		}

		timedOut := false
	collect:
		for {
			select {
			case b, ok := <-chars:
				if !ok { // stdin closed
					fmt.Print("\r\n")
					return false, true
				}
				switch b {
				case 3, 4: // Ctrl+C / Ctrl+D
					fmt.Print("\r\n")
					return false, true
				case '\r', '\n': // Enter — submit
					fmt.Print("\r\n")
					break collect
				case 127, 8: // Backspace / Delete
					if len(input) > 0 {
						input = input[:len(input)-1]
						fmt.Print("\b \b") // erase character on screen
					}
				case 27: // ESC — start of escape sequence (arrow keys, F-keys, …)
					// Pause briefly so the rest of the sequence arrives, then drain.
					time.Sleep(5 * time.Millisecond)
					drainChars(chars)
				default:
					if b >= 32 && b < 127 { // printable ASCII
						input = append(input, b)
						fmt.Printf("%c", b)
					}
				}

			case <-timer:
				// Drain any characters typed right at the boundary so they do
				// not bleed into the next question.
				drainChars(chars)
				timedOut = true
				break collect
			}
		}

		if timedOut {
			fmt.Printf("\r\n    time!  (was: %s)\r\n", expected)
			return false, false
		}

		answer := strings.TrimSpace(string(input))
		if answer == "" {
			// Replay audio and give a fresh timeout window.
			ap.Play(audio)
			continue
		}
		if strings.EqualFold(answer, strings.TrimSpace(expected)) {
			fmt.Print("    correct\r\n")
			return true, false
		}
		fmt.Printf("    wrong  (was: %s)\r\n", expected)
		return false, false
	}
}

// drainChars discards all bytes currently buffered in ch without blocking.
func drainChars(ch <-chan byte) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// NewTerminalKeySource converts a raw byte channel (from startRawInput) into a
// KeyEvent channel.  Because terminal raw mode only delivers key-press events,
// each dit/dah byte triggers a synthetic press+release pair so that the
// IambicAdapter sees a clean edge but will not auto-repeat (single element per
// keystroke).  Control keys (Enter, Delete, Quit) are emitted as press-only
// events.
//
// ditKey and dahKey are the ASCII bytes mapped to dit and dah.  Pass 0 for
// either to disable that mapping (useful in HID mode where those events arrive
// from a separate source).
func NewTerminalKeySource(bytes <-chan byte, ditKey, dahKey byte) <-chan KeyEvent {
	out := make(chan KeyEvent, 64)
	go func() {
		defer close(out)
		now := func() time.Time { return time.Now() }
		press := func(k KeyID) {
			out <- KeyEvent{Key: k, Pressed: true, At: now()}
		}
		pressRelease := func(k KeyID) {
			t := now()
			out <- KeyEvent{Key: k, Pressed: true, At: t}
			out <- KeyEvent{Key: k, Pressed: false, At: t}
		}
		for {
			b, ok := <-bytes
			if !ok {
				press(KeyQuit)
				return
			}
			switch b {
			case 3, 4: // Ctrl+C / Ctrl+D
				press(KeyQuit)
				return
			case '\r', '\n':
				pressRelease(KeyEnter)
			case 127, 8: // Backspace / DEL
				pressRelease(KeyDelete)
			case 27: // ESC — drain the escape sequence silently
				time.Sleep(5 * time.Millisecond)
				drainChars(bytes)
			default:
				if ditKey != 0 && b == ditKey {
					pressRelease(KeyDit)
				} else if dahKey != 0 && b == dahKey {
					pressRelease(KeyDah)
				}
			}
		}
	}()
	return out
}
