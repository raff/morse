package main

import (
	"strings"
	"time"
)

// morseTable maps characters to their ITU Morse code patterns.
var morseTable = map[rune]string{
	'A': ".-", 'B': "-...", 'C': "-.-.", 'D': "-..", 'E': ".",
	'F': "..-.", 'G': "--.", 'H': "....", 'I': "..", 'J': ".---",
	'K': "-.-", 'L': ".-..", 'M': "--", 'N': "-.", 'O': "---",
	'P': ".--.", 'Q': "--.-", 'R': ".-.", 'S': "...", 'T': "-",
	'U': "..-", 'V': "...-", 'W': ".--", 'X': "-..-", 'Y': "-.--",
	'Z': "--..",
	'0': "-----", '1': ".----", '2': "..---", '3': "...--", '4': "....-",
	'5': ".....", '6': "-....", '7': "--...", '8': "---..", '9': "----.",
	'.': ".-.-.-", ',': "--..--", '?': "..--..", '/': "-..-.",
	'-': "-....-", '=': "-...-", '+': ".-.-.", '@': ".--.-.",
}

// ElementKind identifies the type of a Morse element.
type ElementKind int

const (
	KindDit     ElementKind = iota // 1 unit tone
	KindDah                        // 3 unit tone
	KindToneGap                    // 1 unit silence (between tones within a character)
	KindCharGap                    // 3 unit silence (between characters)
	KindWordGap                    // 7 unit silence (between words)
)

// Element is a single Morse code event.
type Element struct{ Kind ElementKind }

// Encode converts a text string into a sequence of Morse elements.
// Gaps are placed between events (not appended after), so CharGap/WordGap
// replace what would otherwise be a ToneGap at a boundary.
func Encode(text string) []Element {
	var out []Element
	words := strings.Fields(strings.ToUpper(text))
	for wi, word := range words {
		if wi > 0 {
			out = append(out, Element{KindWordGap})
		}
		validChars := 0
		for _, ch := range word {
			code, ok := morseTable[ch]
			if !ok {
				continue
			}
			if validChars > 0 {
				out = append(out, Element{KindCharGap})
			}
			validChars++
			for ti, sym := range code {
				if ti > 0 {
					out = append(out, Element{KindToneGap})
				}
				if sym == '.' {
					out = append(out, Element{KindDit})
				} else {
					out = append(out, Element{KindDah})
				}
			}
		}
	}
	return out
}

// Timing holds precomputed durations for a given speed configuration.
type Timing struct {
	Dit     time.Duration
	Dah     time.Duration
	ToneGap time.Duration // gap between tones within a character
	CharGap time.Duration // gap between characters
	WordGap time.Duration // gap between words
}

// NewTiming computes Morse timing using the PARIS standard and, optionally,
// the Farnsworth method.
//
// charWPM sets the speed of the actual dit/dah elements.
//   - 1 dit = 1200/charWPM milliseconds
//
// fwpm sets the perceived (effective) speed by stretching the inter-character
// and inter-word gaps. Must be <= charWPM. Pass 0 or charWPM for standard timing.
//
// Farnsworth math (PARIS = 31 content units + 19 spacing units = 50 total):
//   - content_time  = 31 × dit
//   - total_time    = 60000 / fwpm  ms
//   - spacing_unit  = (total_time − content_time) / 19
//   - char gap      = 3 × spacing_unit
//   - word gap      = 7 × spacing_unit
func NewTiming(charWPM, fwpm int) Timing {
	if fwpm <= 0 || fwpm >= charWPM {
		fwpm = charWPM
	}

	ditMs := 1200.0 / float64(charWPM)

	if fwpm == charWPM {
		dit := floatMs(ditMs)
		return Timing{
			Dit:     dit,
			Dah:     3 * dit,
			ToneGap: dit,
			CharGap: 3 * dit,
			WordGap: 7 * dit,
		}
	}

	totalMs := 60000.0 / float64(fwpm)
	contentMs := 31.0 * ditMs
	spacingUnit := (totalMs - contentMs) / 19.0

	dit := floatMs(ditMs)
	return Timing{
		Dit:     dit,
		Dah:     3 * dit,
		ToneGap: dit,
		CharGap: floatMs(3 * spacingUnit),
		WordGap: floatMs(7 * spacingUnit),
	}
}

func floatMs(ms float64) time.Duration {
	return time.Duration(ms * float64(time.Millisecond))
}
