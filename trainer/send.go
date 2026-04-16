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

// sendWord presents a word for the user to key in Morse code via the iambic
// input pipeline (<-chan MorseInput from NewIambicAdapter).
//
// MorseInputDit/Dah append the corresponding symbol to the current character
// pattern.  A pause of charBoundary (2× CharGap) with no new element flushes
// the accumulated pattern as a decoded character.  MorseInputSubmit submits
// the word for scoring.
//
// ap may be nil (quiet mode); when non-nil each element plays the
// corresponding tone via the queued audio path.
//
// Returns (correct, retried, quit). retried is true if Delete was used at
// least once.
func sendWord(inputs <-chan MorseInput, word string, n int, timing Timing, ap *AudioPlayer) (correct, retried, quit bool) {
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

	// Pre-build tone PCM for per-element audio feedback.
	// Each queued chunk includes the inter-element silence so consecutive
	// elements played via PlayQueued sound like distinct, evenly-spaced tones
	// rather than a continuous buzz.
	var ditTone, dahTone []byte
	if ap != nil {
		gap := silence(timing.ToneGap)
		ditTone = append(tone(ap.freq, ap.volume, timing.Dit), gap...)
		dahTone = append(tone(ap.freq, ap.volume, timing.Dah), gap...)
	}

	var (
		currentPat strings.Builder
		decoded    []rune
		penalized  bool
	)

	// charBoundary: silence after the last element that ends a character.
	// 2× CharGap gives beginners extra time between letters.
	charBoundary := timing.CharGap * 2

	charTimer := time.NewTimer(0)
	drainTimer(charTimer)

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
		case inp, ok := <-inputs:
			if !ok {
				return false, penalized, true
			}
			switch inp.Kind {
			case MorseInputQuit:
				return false, penalized, true

			case MorseInputSubmit:
				flushChar()
				fmt.Print("\r\n")
				goto done

			case MorseInputDelete:
				drainTimer(charTimer)
				currentPat.Reset()
				decoded = decoded[:0]
				penalized = true
				fmt.Print("\r\033[2K    > ")

			case MorseInputDit:
				if ap != nil {
					ap.PlayQueued(ditTone)
				}
				drainTimer(charTimer)
				charTimer.Reset(charBoundary)
				currentPat.WriteByte('.')

			case MorseInputDah:
				if ap != nil {
					ap.PlayQueued(dahTone)
				}
				drainTimer(charTimer)
				charTimer.Reset(charBoundary)
				currentPat.WriteByte('-')
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
