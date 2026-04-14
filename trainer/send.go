package main

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

// reverseTable maps Morse patterns (e.g. ".-") back to their characters.
var reverseTable map[string]rune

func init() {
	reverseTable = make(map[string]rune, len(morseTable))
	for r, pat := range morseTable {
		reverseTable[pat] = r
	}
}

// decodePattern converts a pattern string like ".-" to the corresponding character.
// Returns '?' for unknown patterns.
func decodePattern(pattern string) rune {
	if r, ok := reverseTable[pattern]; ok {
		return r
	}
	return '?'
}

// sendWord presents a word for the user to key in Morse code.
//
// Each press of ditKey appends a dit (.) and each press of dahKey appends a
// dah (-) to the current character pattern. A pause of charBoundary (2× the
// configured CharGap) with no key press flushes the accumulated pattern as a
// decoded character. Enter submits the word for scoring.
//
// ap may be nil (quiet mode); when non-nil each keypress plays the corresponding tone.
//
// Returns (correct, retried, quit). retried is true if Delete was used at least once.
func sendWord(chars <-chan byte, word string, n int, timing Timing, ditKey, dahKey byte, ap *AudioPlayer) (correct, retried, quit bool) {
	upper := strings.ToUpper(strings.TrimSpace(word))
	var expected []rune
	for _, r := range upper {
		if _, ok := morseTable[r]; ok {
			expected = append(expected, r)
		}
	}
	if len(expected) == 0 {
		return false, false, false
	}

	fmt.Printf("[%d] Key: %s\r\n    > ", n, strings.ToLower(word))

	// Pre-build tone PCM for per-keypress audio feedback.
	var ditTone, dahTone []byte
	if ap != nil {
		ditTone = tone(ap.freq, ap.volume, timing.Dit)
		dahTone = tone(ap.freq, ap.volume, timing.Dah)
	}

	var (
		currentPat strings.Builder
		decoded    []rune
		penalized  bool // set when Delete was used; word counts as wrong even if correct
	)

	// charBoundary: gap after the last element that ends the current character.
	// 2× charGap gives beginners extra time to think between elements of a letter.
	charBoundary := timing.CharGap * 2

	// charTimer fires when the user pauses long enough to end a character.
	// Start it unarmed; arm it on the first key press.
	charTimer := time.NewTimer(0)
	if !charTimer.Stop() {
		<-charTimer.C
	}

	flushChar := func() {
		if currentPat.Len() == 0 {
			return
		}
		r := decodePattern(currentPat.String())
		decoded = append(decoded, r)
		fmt.Printf("%c", unicode.ToLower(r))
		currentPat.Reset()
	}

	for {
		select {
		case b, ok := <-chars:
			if !ok {
				return false, penalized, true
			}
			switch b {
			case 3, 4: // Ctrl+C, Ctrl+D
				return false, penalized, true
			case '\r', '\n': // Enter submits the word
				flushChar()
				fmt.Print("\r\n")
				goto done
			case 0x7F, 0x08: // DEL or Backspace — clear and retry
				if !charTimer.Stop() {
					select {
					case <-charTimer.C:
					default:
					}
				}
				currentPat.Reset()
				decoded = decoded[:0]
				penalized = true
				fmt.Print("\r\033[2K    > ")
			default:
				if b == ditKey || b == dahKey {
					// Play audio feedback (non-blocking, GC-safe).
					if ap != nil {
						if b == ditKey {
							ap.PlayQueued(ditTone)
						} else {
							ap.PlayQueued(dahTone)
						}
					}
					// Reset the character boundary timer on each element.
					if !charTimer.Stop() {
						select {
						case <-charTimer.C:
						default:
						}
					}
					charTimer.Reset(charBoundary)
					if b == ditKey {
						currentPat.WriteByte('.')
					} else {
						currentPat.WriteByte('-')
					}
				}
			}
		case <-charTimer.C:
			flushChar()
			// Auto-advance as soon as the correct word is fully keyed.
			if len(decoded) == len(expected) &&
				strings.EqualFold(string(decoded), string(expected)) {
				if penalized {
					fmt.Print("\r\n    correct (retried)\r\n")
					return false, true, false
				}
				fmt.Print("\r\n    correct\r\n")
				return true, false, false
			}
		}
	}

done:
	sentStr := string(decoded)
	expectedStr := string(expected)
	typed := strings.EqualFold(sentStr, expectedStr)
	correct = typed && !penalized
	switch {
	case typed && penalized:
		fmt.Print("    correct (retried)\r\n")
	case typed:
		fmt.Print("    correct\r\n")
	default:
		fmt.Printf("    wrong  (was: %s)\r\n", strings.ToLower(expectedStr))
	}
	return correct, penalized, false
}
