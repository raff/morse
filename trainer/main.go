package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// builtinWords is a set of common English words suitable for CW practice.
var builtinWords = []string{
	"the", "of", "and", "to", "a", "in", "that", "is", "was", "he",
	"for", "it", "with", "as", "his", "on", "be", "at", "by", "this",
	"had", "not", "are", "but", "from", "or", "an", "they", "which", "one",
	"you", "were", "her", "all", "she", "there", "would", "their", "we", "him",
	"been", "has", "when", "who", "will", "more", "no", "if", "out", "so",
	"said", "what", "up", "its", "about", "into", "than", "them", "can", "only",
	"other", "new", "some", "could", "time", "these", "two", "may", "then", "do",
	"first", "any", "now", "such", "like", "our", "over", "man", "me", "even",
	"most", "after", "also", "how", "your", "word", "long", "good", "see", "come",
}

func loadWordList(filename string) ([]string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var words []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		words = append(words, line)
	}
	return words, sc.Err()
}

func main() {
	charWPM := flag.Int("wpm", 20, "character speed in WPM (sets dit/dah duration)")
	fwpm := flag.Int("fwpm", 0, "Farnsworth effective speed in WPM (0 = same as -wpm)\n\t  must be <= -wpm; stretches char/word gaps to slow perceived speed")
	freq := flag.Float64("freq", 700, "tone frequency in Hz")
	volume := flag.Float64("vol", 0.5, "volume (0.0–1.0)")
	file := flag.String("file", "", "word list file: one word or phrase per line, # for comments")
	count := flag.Int("count", 0, "number of entries to play (0 = play all once)")
	shuffle := flag.Bool("shuffle", false, "randomize playback order")
	repeat := flag.Bool("repeat", false, "loop through the list indefinitely")
	show := flag.Bool("show", false, "print each entry before playing (read-along mode)")
	reveal := flag.Bool("reveal", false, "print each entry after playing (decode-then-check mode)")
	check := flag.Bool("check", false, "quiz mode: type what you heard, Enter alone to replay")
	checkTimeout := flag.Duration("timeout", 0, "in -check mode: time limit per entry (e.g. 3s); 0 = no limit")
	gap := flag.Duration("gap", 0, "extra silence between entries (e.g. 500ms, 2s)")
	send := flag.Bool("send", false, "sending mode: display a word, key it in Morse")
	showStats := flag.Bool("stats", false, "show session history and quit")
	ditKeyStr := flag.String("dit-key", "[", "dit key in sending mode (single character)")
	dahKeyStr := flag.String("dah-key", "]", "dah key in sending mode (single character)")
	quiet := flag.Bool("quiet", false, "in -send mode: skip audio playback of the word")
	flag.Parse()

	// Stats mode: no audio or word list needed.
	if *showStats {
		printStats()
		return
	}

	// needAudio is false only when the user explicitly silenced send mode.
	needAudio := !(*send && *quiet)

	// Parse dit/dah keys.
	if len(*ditKeyStr) != 1 || len(*dahKeyStr) != 1 {
		log.Fatal("-dit-key and -dah-key must each be a single ASCII character")
	}
	ditKey := (*ditKeyStr)[0]
	dahKey := (*dahKeyStr)[0]

	// Validate / resolve Farnsworth speed.
	if *fwpm == 0 {
		*fwpm = *charWPM
	}
	if *fwpm > *charWPM {
		fmt.Fprintf(os.Stderr, "warning: -fwpm %d > -wpm %d; Farnsworth ignored\n", *fwpm, *charWPM)
		*fwpm = *charWPM
	}
	if *volume < 0 || *volume > 1 {
		log.Fatal("-vol must be between 0.0 and 1.0")
	}

	timing := NewTiming(*charWPM, *fwpm)

	fmt.Printf("Speed : %d WPM char / %d WPM effective\n", *charWPM, *fwpm)
	fmt.Printf("Tone  : %.0f Hz  vol %.0f%%\n", *freq, *volume*100)
	fmt.Printf("Timing: dit=%-6v dah=%-6v char-gap=%-6v word-gap=%v\n\n",
		timing.Dit.Round(time.Millisecond),
		timing.Dah.Round(time.Millisecond),
		timing.CharGap.Round(time.Millisecond),
		timing.WordGap.Round(time.Millisecond),
	)
	if *check {
		if *checkTimeout > 0 {
			fmt.Printf("Quiz mode: %v per entry. Enter alone to replay.\n\n", *checkTimeout)
		} else {
			fmt.Print("Quiz mode: type what you heard and press Enter. Enter alone to replay.\n\n")
		}
	}
	if *send {
		fmt.Printf("Send mode: %s=dit  %s=dah  (also left-Ctrl=dit  right-Ctrl=dah), hold to auto-repeat.\n\n",
			*ditKeyStr, *dahKeyStr)
	}

	// Load word list: command-line args > -file > built-in.
	var words []string
	switch {
	case len(flag.Args()) > 0:
		words = flag.Args()
	case *file != "":
		var err error
		words, err = loadWordList(*file)
		if err != nil {
			log.Fatalf("loading word list: %v", err)
		}
		fmt.Printf("Loaded %d entries from %s\n\n", len(words), *file)
	default:
		words = builtinWords
		fmt.Printf("Using built-in word list (%d words). Use -file to load your own.\n\n", len(words))
	}
	if len(words) == 0 {
		log.Fatal("word list is empty")
	}

	// Determine how many to play.
	total := *count
	if total == 0 && !*repeat {
		total = len(words)
	}

	// Initialize audio unless the user silenced send mode.
	var err error
	var ap *AudioPlayer
	if needAudio {
		ap, err = NewAudioPlayer(*freq, *volume)
		if err != nil {
			log.Fatalf("audio init failed: %v\n(oto requires an audio output device)", err)
		}
		defer ap.Close()
	}

	// Set up raw input for check and send modes. Raw mode disables OPOST so
	// output in the quiz/send loop uses \r\n. The terminal is restored before
	// printing the final summary so that uses normal \n.
	var stdinChars <-chan byte
	var restoreTerm func()
	if *check || *send {
		stdinChars, restoreTerm, err = startRawInput()
		if err != nil {
			log.Fatalf("raw terminal: %v", err)
		}
	}

	// Set up the iambic input pipeline for send mode.
	//
	// On macOS, StartSendKeySource uses CGEventTap to capture real key
	// press+release events for both the configured paddle keys ([/]) and
	// left/right Control (commonly emitted by USB iambic paddles).  The
	// IambicAdapter converts held keys into WPM-rate auto-repeating elements.
	//
	// On other platforms StartSendKeySource wraps the raw terminal byte stream;
	// press+release is synthetic so there is no hold-to-repeat.
	var morseInputs <-chan MorseInput
	var stopSend func()
	if *send {
		keyEvents, stop, keyErr := StartSendKeySource(stdinChars, ditKey, dahKey)
		if keyErr != nil {
			log.Fatalf("send input: %v", keyErr)
		}
		stopSend = stop
		morseInputs = NewIambicAdapter(keyEvents, timing)
	}

	// Signal handler. In raw mode Ctrl+C arrives as byte 3 (ISIG is disabled),
	// so we only need the signal handler for SIGTERM and for non-check/send SIGINT.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		if stopSend != nil {
			stopSend()
		}
		if restoreTerm != nil {
			restoreTerm()
		}
		fmt.Println("\nInterrupted.")
		os.Exit(0)
	}()

	var (
		played       int
		correct      int
		retried      int
		wrongAnswers []string
	)

	startTime := time.Now()

outer:
	for {
		// Build (optionally shuffled) index for this pass.
		indices := make([]int, len(words))
		for i := range indices {
			indices[i] = i
		}
		if *shuffle {
			rand.Shuffle(len(indices), func(i, j int) {
				indices[i], indices[j] = indices[j], indices[i]
			})
		}

		for _, idx := range indices {
			if total > 0 && played >= total {
				break outer
			}

			entry := words[idx]
			els := Encode(entry)
			if len(els) == 0 {
				continue // no encodable characters
			}

			var audio []byte
			if needAudio {
				audio = BuildAudio(els, timing, *freq, *volume)
			}

			if *show && !*check && !*send {
				fmt.Printf("[%d] %s\n", played+1, entry)
			}

			if needAudio && !*send {
				ap.Play(audio)
			}

			switch {
			case *check:
				hit, quit := askUser(stdinChars, ap, audio, entry, played+1, *checkTimeout)
				if quit {
					break outer
				}
				if hit {
					correct++
				} else {
					wrongAnswers = append(wrongAnswers, entry)
				}
				played++
				// \r\n because we are still in raw mode here.
				fmt.Printf("    %d/%d (%d%%)\r\n", correct, played, 100*correct/played)

			case *send:
				hit, didRetry, quit := sendWord(morseInputs, entry, played+1, timing, ap)
				if quit {
					break outer
				}
				if hit {
					correct++
				} else {
					wrongAnswers = append(wrongAnswers, entry)
				}
				if didRetry {
					retried++
				}
				played++
				fmt.Printf("    %d/%d (%d%%)\r\n", correct, played, 100*correct/played)

			case *reveal:
				fmt.Printf("[%d] %s\n", played+1, entry)
				played++

			default:
				played++
			}

			if *gap > 0 {
				time.Sleep(*gap)
			}
		}

		if !*repeat {
			break
		}
	}

	// Stop the send key source before restoring the terminal.
	if stopSend != nil {
		stopSend()
	}

	// Restore terminal before printing the summary so \n works normally.
	if restoreTerm != nil {
		restoreTerm()
	}

	// Persist session results for -stats.
	if (*check || *send) && played > 0 {
		mode := "check"
		if *send {
			mode = "send"
		}
		if err := appendSession(SessionRecord{
			Date:     time.Now(),
			Mode:     mode,
			Words:    played,
			Correct:  correct,
			Retried:  retried,
			WPM:      *charWPM,
			Duration: time.Since(startTime).Milliseconds(),
		}); err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save session: %v\n", err)
		}
	}

	printSummary(played, correct, wrongAnswers, *check || *send)
}

func printSummary(played, correct int, wrongAnswers []string, checkMode bool) {
	fmt.Println()
	if !checkMode || played == 0 {
		fmt.Printf("Done. Played %d %s.\n", played, pluralize("entry", "entries", played))
		return
	}
	fmt.Printf("Final score: %d/%d (%d%%)\n", correct, played, 100*correct/played)
	if len(wrongAnswers) > 0 && len(wrongAnswers) <= 20 {
		fmt.Printf("Missed: %s\n", strings.Join(wrongAnswers, ", "))
	} else if len(wrongAnswers) > 20 {
		fmt.Printf("Missed: %d entries\n", len(wrongAnswers))
	}
}
