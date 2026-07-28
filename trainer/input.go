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
//
// askUser owns playback: the prompt is printed first and the audio starts in
// the background, so the user can see where to type and can answer while the
// entry is still sounding. Characters arrive one byte at a time and are echoed
// as they are typed.
//
// The timeout only starts once playback has finished, so -timeout measures
// thinking time rather than including the length of the entry.
// An empty submission replays the audio and resets the timeout.
// Returns (correct, quit).
func askUser(chars <-chan byte, ap *AudioPlayer, audio []byte, expected string, n int, timeout time.Duration) (correct, quit bool) {
	// Discard anything left over from the previous entry (for example keys hit
	// while the score line was printing) so this answer starts clean. Input
	// typed from here on belongs to this entry and is kept.
	flushChars(chars)

	for {
		fmt.Printf("[%d] > ", n)
		var input []byte

		playDone, stopPlay := ap.PlayAsync(audio)

		// A nil channel blocks forever. timer stays nil until playback ends
		// (and forever if no timeout was requested).
		var timer <-chan time.Time

		timedOut, quitting := false, false
	collect:
		for {
			select {
			case <-playDone:
				// Playback finished — start the clock and stop selecting on
				// this channel (a closed channel is always ready).
				playDone = nil
				if timeout > 0 {
					timer = time.After(timeout)
				}

			case b, ok := <-chars:
				if !ok { // stdin closed
					quitting = true
					break collect
				}
				switch b {
				case 3, 4: // Ctrl+C / Ctrl+D
					quitting = true
					break collect
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
						fmt.Printf("%c", b) // raw mode disables echo; echo here
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

		// Silence the entry before printing the verdict or moving on, so two
		// entries never overlap.
		stopPlay()

		if quitting {
			fmt.Print("\r\n")
			return false, true
		}
		if timedOut {
			fmt.Printf("\r\n    time!  (was: %s)\r\n", expected)
			return false, false
		}

		answer := strings.TrimSpace(string(input))
		if answer == "" {
			// Replay: the next loop iteration restarts the audio and, once it
			// finishes, gives a fresh timeout window.
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

// flushChars discards buffered input, pausing briefly so bytes still in flight
// between the stdin reader goroutine and the channel are caught too.
func flushChars(ch <-chan byte) {
	drainChars(ch)
	time.Sleep(2 * time.Millisecond)
	drainChars(ch)
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
