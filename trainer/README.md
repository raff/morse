# mt — Morse code trainer

A command-line Morse code trainer written in Go. It plays audio of words or
phrases in Morse code and optionally quizzes you on what you heard.

## Build

macOS requires the external linker to satisfy `dyld`'s `LC_UUID` requirement
(a macOS 26 Tahoe stricter-linker change). The `Makefile` wraps this:

```
make build        # produces ./mt
make run ARGS="…" # build + run with arguments
```

Or build directly:

```
go build -ldflags="-linkmode=external" -o mt .
```

## Usage

```
./mt [flags] [word|phrase …]
```

Word sources are checked in order: **command-line arguments → `-file` → built-in list**.

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-wpm N` | 20 | Character speed in WPM. Sets the duration of every dit and dah. |
| `-fwpm N` | same as `-wpm` | Farnsworth effective speed in WPM. Must be ≤ `-wpm`. Stretches inter-character and inter-word gaps so characters are sent at full speed but perceived speed is slower. |
| `-freq F` | 700 | Tone frequency in Hz. |
| `-vol F` | 0.5 | Volume, 0.0–1.0. |
| `-file path` | — | Word list file. One word or phrase per line; `#` starts a comment. |
| `-count N` | 0 | Number of entries to play. 0 means play all entries once. |
| `-shuffle` | off | Randomize playback order each pass. |
| `-repeat` | off | Loop through the list indefinitely. |
| `-show` | off | Print each entry to the terminal *before* playing (read-along mode). Ignored in `-check` mode. |
| `-reveal` | off | Print each entry to the terminal *after* playing (decode-then-check mode). |
| `-check` | off | Quiz mode: type what you heard after each entry. Enter alone replays. Requires an interactive terminal. |
| `-timeout d` | 0 | In `-check` mode: time limit per entry, e.g. `3s`. 0 means no limit. |
| `-gap d` | 0 | Extra silence between entries, e.g. `500ms` or `2s`. Added on top of the normal inter-word gap. |

### Examples

```bash
# Play the built-in word list once at 20 WPM
./mt

# Play specific words from the command line
./mt the quick brown fox

# Play phrases (quote multi-word entries)
./mt "cq cq de w1aw" "qrz?" "tu 73"

# 20 WPM character speed, 8 WPM effective (Farnsworth), shuffled, quiz mode
./mt -wpm 20 -fwpm 8 -shuffle -check

# Quiz with a 3-second time limit per word
./mt -check -timeout 3s -shuffle

# Load a custom list, loop forever, reveal each answer after playing
./mt -file callsigns.txt -repeat -reveal

# High speed practice, 30 WPM, 600 Hz tone
./mt -wpm 30 -freq 600 -shuffle -repeat

# Quiz on a fixed set of words, 10 at a time
./mt -check -shuffle -count 10 the and of to a in that is was for
```

### Quiz mode (`-check`)

The terminal is put in raw mode so keystrokes are processed as they arrive.
After each entry plays you will see a prompt:

```
[3] > 
```

- Type what you heard — characters appear as you type, Backspace works.
- Press **Enter** to submit your answer.
- Press **Enter** alone (empty input) to replay the same entry. If `-timeout`
  is set, replaying resets the timer.
- Press **Ctrl+C** or **Ctrl+D** to stop early.

After each answer the running score is shown:

```
[1] > the
    correct
    1/1 (100%)
[2] > wourld
    wrong  (was: world)
    1/2 (50%)
[3] > 
    time!  (was: would)
    1/3 (33%)
```

At the end a final summary is printed:

```
Final score: 8/10 (80%)
Missed: would, come
```

### Word list file format

```
# common English words
the
of
and

# ham radio phrases
cq cq de w1aw
qrz?
tu 73 de w1aw
```

Blank lines and lines beginning with `#` are ignored.

## Implementation

The program is split across four source files.

### `morse.go` — encoding and timing

**`morseTable`** maps every supported character to its ITU pattern string
(`.` = dit, `-` = dah). Supported: A–Z, 0–9, and the punctuation
`. , ? / - = + @`.

**`Encode(text string) []Element`** converts a string to a flat slice of
`Element` values, one per event:

- `KindDit` / `KindDah` — a tone of 1 or 3 units.
- `KindToneGap` — 1 unit of silence between tones within the same character.
- `KindCharGap` — 3 units of silence between characters.
- `KindWordGap` — 7 units of silence between words.

Gaps are placed *between* events rather than appended after each tone, so a
`CharGap` or `WordGap` directly replaces what would otherwise be a `ToneGap`
at a boundary. Unknown characters are silently skipped.

**`NewTiming(charWPM, fwpm int) Timing`** computes all five durations.

Standard timing uses the PARIS standard: the word PARIS contains exactly 50
dit-units, so at *N* WPM one dit lasts `1200/N` milliseconds. All other
durations follow from the standard ratios (1 : 3 : 1 : 3 : 7).

The **Farnsworth method** keeps dit/dah at the `charWPM` rate but stretches
the inter-character and inter-word gaps so the overall throughput matches
`fwpm`. The math derives from the fact that PARIS contains 31 *content* units
(tones + intra-character gaps) and 19 *spacing* units (inter-character and
inter-word gaps):

```
dit            = 1200 / charWPM  ms
content_time   = 31 × dit
total_time     = 60000 / fwpm  ms
spacing_unit   = (total_time − content_time) / 19
char gap       = 3 × spacing_unit
word gap       = 7 × spacing_unit
```

When `fwpm == charWPM` the spacing unit equals one dit and the result is
identical to standard timing.

### `audio.go` — PCM generation and playback

Uses **[ebitengine/oto v3](https://github.com/ebitengine/oto)** for audio
output. On macOS this drives CoreAudio via `ebitengine/purego`.

**`BuildAudio(elements, timing, freq, volume) []byte`** walks the element
slice and concatenates byte slices for each event into one PCM buffer
(signed 16-bit LE, mono, 44 100 Hz). A full word-gap of silence is appended
at the end so that consecutive entries played back-to-back have proper spacing
without any additional logic in the caller.

**`tone(freq, volume, dur) []byte`** generates a sine wave for the given
duration. A 5 ms linear **attack and release envelope** is applied at each
end to eliminate the audible click that a hard-started or hard-stopped tone
would produce. The envelope ramps from 0 to full amplitude over 5 ms, and
back to 0 over the last 5 ms.

**`AudioPlayer.Play(data []byte)`** creates an oto player for each buffer,
starts it, and polls `IsPlaying()` every millisecond until the buffer is
exhausted, making the call synchronous from the caller's perspective.

### `input.go` — raw terminal input and quiz interaction

**`startRawInput()`** uses `golang.org/x/term` to put the terminal in raw
mode (`ICANON=0`, `ECHO=0`, `ISIG=0`, `OPOST=0`) so bytes arrive the instant
a key is pressed rather than after Enter. A goroutine reads one byte at a time
from stdin into a buffered channel, leaving the quiz loop free to `select`
between input and a timer without any blocking reads.

**`askUser(..., timeout)`** manages one quiz entry:

1. Prints the `[N] > ` prompt and creates a `time.After` timer (nil when
   timeout is 0, which blocks forever — effectively disabling it).
2. Loops on a `select` between the byte channel and the timer:
   - Printable ASCII is appended to the answer buffer and echoed.
   - Backspace (`0x7f`/`0x08`) removes the last byte and erases it on screen
     with the `\b \b` sequence.
   - ESC (`0x1b`) drains the channel after a 5 ms pause to discard the
     remainder of any escape sequence (arrow keys, F-keys, etc.).
   - Ctrl+C (`0x03`) and Ctrl+D (`0x04`) return `quit=true`.
   - Enter breaks out of the collect loop.
   - Timer expiry calls `drainChars` to discard any bytes typed at the
     boundary, then returns `correct=false`.
3. An empty submission replays the audio and restarts from step 1 with a
   fresh timer window.

Because `term.MakeRaw` disables `OPOST`, `\n` no longer implies a carriage
return. All output inside `askUser` and the score line in `main.go` use `\r\n`
explicitly. The terminal is restored via `restoreTerm()` before the final
summary so that output returns to normal.

In raw mode Ctrl+C does not generate SIGINT (ISIG is off), so it is caught
directly as byte `0x03`. A separate goroutine handles `SIGTERM` (and
non-check-mode `SIGINT`) and also restores the terminal before exiting.

### `main.go` — CLI and playback loop

Parses flags with the standard `flag` package. Word source priority:
command-line arguments → `-file` → built-in 90-word list.

The main loop builds a (optionally shuffled) index each pass, then iterates
through entries. For each entry it:

1. Encodes the text to elements (`Encode`).
2. Renders elements to PCM (`BuildAudio`).
3. Plays the audio (`AudioPlayer.Play`).
4. In `-check` mode, calls `askUser`, then prints the running score.
5. In `-reveal` mode, prints the entry text after playing.

## Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/ebitengine/oto/v3` | v3.4.0 | Audio output (CoreAudio on macOS) |
| `github.com/ebitengine/purego` | v0.9.0 | CGo-free Objective-C / dylib bridge (indirect) |
| `golang.org/x/term` | v0.42.0 | Raw terminal mode and state restore |
| `golang.org/x/sys` | v0.43.0 | Low-level OS interfaces (indirect) |
