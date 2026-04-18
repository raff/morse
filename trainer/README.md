# mt — Morse code trainer

A command-line Morse code trainer written in Go. It plays audio of words or
phrases in Morse code and optionally quizzes you on what you heard, or
prompts you to key each word yourself in Morse.

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
| `-show` | off | Print each entry to the terminal *before* playing (read-along mode). Ignored in `-check` and `-send` modes. |
| `-reveal` | off | Print each entry to the terminal *after* playing (decode-then-check mode). |
| `-check` | off | Quiz mode: type what you heard after each entry. Enter alone replays. Requires an interactive terminal. |
| `-timeout d` | 0 | In `-check` mode: time limit per entry, e.g. `3s`. 0 means no limit. |
| `-gap d` | 0 | Extra silence between entries, e.g. `500ms` or `2s`. Added on top of the normal inter-word gap. |
| `-send` | off | Sending mode: displays each word and waits for you to key it in Morse. |
| `-dit-key c` | `[` | Key used for dit in sending mode (single ASCII character). |
| `-dah-key c` | `]` | Key used for dah in sending mode (single ASCII character). |
| `-quiet` | off | In `-send` mode: skip audio playback of each keypress. |
| `-stats` | off | Show session history and quit. No word list or audio needed. |

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

# Sending practice: key each displayed word using [ (dit) and ] (dah)
./mt -send -shuffle

# Sending practice with custom keys and Farnsworth timing
./mt -send -dit-key z -dah-key x -wpm 20 -fwpm 12

# Sending practice, silent (no keypress audio feedback)
./mt -send -quiet

# Show session history
./mt -stats
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

### Sending mode (`-send`)

The terminal is put in raw mode. For each entry the word is displayed and you
key it in Morse using the dit and dah keys (`[` and `]` by default). Each
keypress plays an audio tone of the correct dit or dah duration. A pause of
2× the character gap (360 ms at 20 WPM) with no keypresses ends the current
character and decodes it.

```
[1] Key: the
    > the
    correct
    1/1 (100%)
[2] Key: of
    > oe
    wrong  (was: of)
    1/2 (50%)
```

- The decoded characters appear as you type.
- **Hold** a dit or dah key to auto-repeat at the configured WPM rate —
  useful for practicing smooth, evenly-timed elements.
- Pressing the other paddle while one is held pauses the first paddle's
  auto-repeat (iambic keyer behaviour).
- On **macOS**, **left-Ctrl** and **right-Ctrl** can be used as dit and dah
  respectively. This lets you plug in a USB iambic paddle that presents itself
  as a keyboard with Ctrl keys. Hold-to-repeat and iambic behaviour work the
  same way as with `[`/`]`.
- Once the correct word is fully keyed and the character boundary timer fires,
  the entry advances automatically — no Enter needed.
- Press **Enter** at any time to submit what you have so far.
- Press **Backspace** or **Delete** to clear the current attempt and retry.
  A retried word is scored as wrong even if you subsequently key it correctly.
- Press **Ctrl+C** or **Ctrl+D** to stop early.

Session results (including retried-word count) are saved to `~/.mt/sessions.jsonl`.

### Stats mode (`-stats`)

```
./mt -stats
```

Reads all saved session records and prints a summary:

```
Sessions: 12 total (8 receive, 4 send)
Accuracy: 74% overall (182/246 correct)
Retried : 17 words used Delete
WPM     : 15 → 15 → 20 → 20 → 20 → 20 → 25 → 25 (last 8)

Recent sessions:
  2025-04-13  send    18/25 ( 72%)  20 wpm  4m12s  3 retried
  2025-04-12  receive 24/30 ( 80%)  20 wpm  3m48s
  ...
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

The program is split across nine source files.

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
would produce.

**`AudioPlayer.Play(data []byte)`** creates an oto player, starts it, and
polls `IsPlaying()` every millisecond until the buffer is exhausted. Used for
word playback in listen/quiz modes where blocking is acceptable.

**`AudioPlayer.PlayQueued(data []byte)`** enqueues a PCM chunk for serial
playback via a background goroutine and returns immediately. Used in sending
mode so that rapid dit/dah keypresses each play a clean, gap-terminated tone
in strict sequence with no overlap and no blocking of the input loop.

**`AudioPlayer.PlayNoWait(data []byte)`** creates an oto player, starts it,
and returns immediately. The player reference is kept in `AudioPlayer.active`
until `IsPlaying()` returns false, preventing Go's garbage collector from
collecting (and thereby stopping) the player mid-play.

### `input.go` — raw terminal input and quiz interaction

**`startRawInput()`** uses `golang.org/x/term` to put the terminal in raw
mode (`ICANON=0`, `ECHO=0`, `ISIG=0`, `OPOST=0`) so bytes arrive the instant
a key is pressed rather than after Enter. A goroutine reads one byte at a time
from stdin into a buffered channel, leaving the quiz loop free to `select`
between input and a timer without any blocking reads.

**`NewTerminalKeySource(bytes <-chan byte, ditKey, dahKey byte) <-chan KeyEvent`**
converts the raw byte stream into a `KeyEvent` channel. Each dit/dah byte
emits a synthetic press immediately followed by a release. Used as the
non-macOS fallback and for non-send modes where press/release timing is not
needed.

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
non-check/send-mode `SIGINT`) and also restores the terminal before exiting.

### `keyevent.go` — iambic input pipeline

Defines the two-stage event model for sending mode.

**`KeyEvent`** carries a raw key press or release with a timestamp. `KeyID`
values: `KeyDit`, `KeyDah`, `KeyEnter`, `KeyDelete`, `KeyQuit`.

**`MorseInput`** is the processed event consumed by `sendWord`. `MorseInputKind`
values: `MorseInputDit`, `MorseInputDah`, `MorseInputDelete`, `MorseInputSubmit`,
`MorseInputQuit`.

**`NewIambicAdapter(keys <-chan KeyEvent, timing Timing) <-chan MorseInput`**
runs a goroutine that converts raw press/release events into WPM-rate
auto-repeating Morse elements:

- **Press**: emit one element immediately, arm a repeat timer at the WPM period
  (`dit + toneGap` or `dah + toneGap`).
- **Repeat timer fires** (key still held, other paddle not held): emit again
  and re-arm.
- **Other paddle pressed**: drain the current paddle's repeat timer to prevent
  simultaneous timers generating spurious extra elements (e.g. `r` instead of
  `a`). When the other paddle is released, restart the paused timer.
- **Release**: cancel the repeat timer.

**Contact-bounce debounce**: press events are rejected if the previous accepted
event for that key arrived within 15 ms. Releases are never debounced so that
rapid simultaneous keying cannot leave a paddle stuck held.

**Squeeze-delete**: holding both paddles simultaneously for 1.5 s emits a
`MorseInputDelete`, clearing the current input without reaching for the
keyboard. While a squeeze is in progress, individual paddle releases do not
cancel the timer or restart the other paddle's auto-repeat — only releasing
both paddles together cancels the squeeze.

A priority-peek on the key channel is performed when a timer fires, so that a
key event arriving at the same instant as a tick is processed first.

### `input_send_darwin.go` / `input_send_other.go` — platform key sources

**`StartSendKeySource(stdinChars <-chan byte, ditKey, dahKey byte) (<-chan KeyEvent, func(), error)`**
is the platform-specific entry point for sending-mode input.

**On macOS** (`input_send_darwin.go`), a `CGEventTap` intercepts system-wide
keyboard events (requires Accessibility permission). This gives accurate
press **and** release timestamps for both the configured dit/dah keys and
left/right Control (commonly emitted by USB iambic paddles).

Two mechanisms scope events to our own terminal window:

- **Stdin correlation** (regular keys): the tap records the keydown timestamp
  as *pending*. When the corresponding byte arrives in our PTY stdin within
  30 ms, the press is *confirmed* and delivered. Events typed in other windows
  never produce a byte in our stdin, so their pending entries simply expire.
- **Terminal focus events** (modifier keys): on startup `\033[?1004h` enables
  DECSET 1004 reporting. The terminal sends `ESC [ I` to our stdin when our
  window gains focus and `ESC [ O` when it loses focus. The tap callback
  checks a `g_windowFocused` flag before delivering Ctrl events, so paddle
  presses in other windows (including other windows of the same Terminal.app)
  are silently dropped. Focus reporting is disabled on exit with `\033[?1004l`.

OS key-repeat events are suppressed (`kCGKeyboardEventAutorepeat`); timing is
driven entirely by the `IambicAdapter`.

**On other platforms** (`input_send_other.go`), the raw byte stream is wrapped
by `NewTerminalKeySource`: each dit/dah byte emits a synthetic press+release
pair. Hold-to-repeat is not available.

### `send.go` — sending mode

**`decodePattern(pattern string) rune`** maps a Morse pattern string (e.g.
`".-"`) back to its character using a reverse lookup table built from
`morseTable` at startup.

**`sendWord(inputs <-chan MorseInput, ...) (correct, retried, quit bool)`**
manages one sending entry:

1. Displays `[N] Key: word` and an empty `>` prompt.
2. Pre-builds dit and dah PCM chunks (tone + inter-element silence) for queued
   audio feedback.
3. Runs a `select` loop over the `MorseInput` channel and a character boundary
   timer:
   - `MorseInputDit` / `MorseInputDah`: append `.` or `-` to the current
     character pattern, play the corresponding tone via `PlayQueued`, and reset
     the character boundary timer.
   - `MorseInputDelete`: clears the entire attempt and sets a `penalized` flag.
   - `MorseInputSubmit`: flushes the current character and jumps to scoring.
   - `MorseInputQuit`: returns `quit=true`.
4. When the character boundary timer fires (2× `CharGap`, 360 ms at 20 WPM),
   the current pattern is decoded and appended to the decoded word. If the
   decoded word matches the target, the entry advances automatically.
5. Returns `correct=true` only if the word matched **and** no Delete was used.
   `retried=true` if Delete was pressed at least once.

### `stats.go` — session persistence

**`appendSession(SessionRecord)`** appends a JSON line to `~/.mt/sessions.jsonl`,
creating the directory if needed.

**`loadSessions()`** reads all records from the file, skipping malformed lines.

**`printStats()`** aggregates totals, computes accuracy, and prints a WPM
trend (last 8 sessions) and a recent-session table (last 5, newest first).

### `main.go` — CLI and playback loop

Parses flags with the standard `flag` package. Word source priority:
command-line arguments → `-file` → built-in 90-word list.

`-stats` mode runs `printStats()` and exits before any audio or word-list
setup. Otherwise, the main loop builds a (optionally shuffled) index each
pass, then iterates through entries. For each entry it:

1. Encodes the text to elements (`Encode`).
2. Renders elements to PCM (`BuildAudio`), unless in `-send` with `-quiet`.
3. In listen modes, plays the audio (`AudioPlayer.Play`).
4. Dispatches to the appropriate mode handler:
   - `-check`: `askUser`, then prints the running score.
   - `-send`: `sendWord`, then prints the running score.
   - `-reveal`: prints the entry text after playing.

At the end, if any entries were attempted in `-check` or `-send` mode, the
session record is appended to `~/.mt/sessions.jsonl`.

## Dependencies

| Module | Version | Purpose |
|--------|---------|---------|
| `github.com/ebitengine/oto/v3` | v3.4.0 | Audio output (CoreAudio on macOS) |
| `github.com/ebitengine/purego` | v0.9.0 | CGo-free Objective-C / dylib bridge (indirect) |
| `golang.org/x/term` | v0.42.0 | Raw terminal mode and state restore |
| `golang.org/x/sys` | v0.43.0 | Low-level OS interfaces (indirect) |
