# TODOS

## High priority

### USB iambic paddle debounce — ongoing refinement
**What:** Contact bounce on paddle contacts generates spurious release+press or press+release pairs within ~5 ms, causing extra elements or a stuck-held loop.
**Current approach:** `runIambic` debounces PRESSES only (not releases): a press is ignored if it arrives within 15 ms of the previous accepted dit/dah event (press or release). Releases are always accepted immediately so that quick simultaneous keying (press dit, press dah, release dit all within 15 ms) cannot get stuck.
**Status:** Decoding is now correct. Minor remaining issue: first dit of a word sometimes has an audio stutter (sounds like `. .-.` instead of `..-. `), but the character displays correctly as F — see audio warmup TODO below.
**Tuning:** If spurious elements return, run `make timing` while keying `f` and check whether the bounce interval exceeds 15 ms. Raise `debounceDuration` if needed (max safe value ≈ half the shortest real press-to-press gap).
**Depends on:** `IambicAdapter` + `CGEventTap` pipeline (current code).

### Audio warmup latency on first element
**What:** The first dit of the first word in a session sometimes sounds slightly late, making a 4-element character (e.g. `f` = `..-. `) sound like it has an extra gap after the first element.
**Why:** oto audio players may have per-player startup overhead. The `drainQueue` goroutine creates a new `oto.Player` for each tone; the first one pays the OS audio pipeline warmup cost.
**Possible fix:** In `sendWord` (or `NewAudioPlayer`), play a 1–5 ms silent buffer before keying starts to pre-warm the audio pipeline.
**Depends on:** Audio subsystem (`audio.go`, oto library).



### Add unit tests for pure functions
**What:** Create morse_test.go, audio_test.go, main_test.go, stats_test.go, send_test.go.
**Why:** Zero tests exist. A bug in Encode() or decodeKey() corrupts every score in send mode silently.
**Pros:** Catches regressions, documents expected behavior, makes future feature additions safe.
**Cons:** ~15min with CC+gstack.
**Context:** All testable functions are pure (no terminal, no audio device). Use standard Go testing: `go test ./...`. Test files live alongside source in the trainer/ directory. Key test targets: Encode(), NewTiming(), BuildAudio(), loadWordList(), decodeKey(), appendSession(), loadSessions(), aggregateStats().
**Depends on:** None — can start immediately.

## After core features ship

### GitHub Actions release pipeline
**What:** Workflow that builds and publishes mt binaries on tag push. Targets: linux-amd64, linux-arm64, darwin-amd64, darwin-arm64.
**Why:** Current install requires `git clone && make build`. Barrier for ham radio operators who just want to run the tool.
**Pros:** Go cross-compiles trivially (`GOOS=linux GOARCH=amd64 go build`). One `.github/workflows/release.yml` file.
**Cons:** Only useful if sharing publicly. Not urgent for personal use.
**Context:** Flag this before Phase 4 (QSO sim) when the tool is complete enough to share broadly. See design doc for the four-phase plan.
**Depends on:** stats + send features complete. ✓ Both shipped.
