# TODOS

## High priority

### Fix send-mode dit auto-repeat (USB iambic paddle)
**What:** Consecutive dit presses consistently produce too many dits. Typing `f` (..-.) reliably generates extra dits (e.g. `...-.`). The problem does not reproduce in other trainers at the same WPM, so it is not operator error.
**Why:** Corrupts character decoding silently; makes the trainer unusable for practice at any realistic keying speed.
**What we know:** Both dit and dah now use `firstRepeat = CharGap + 2×ToneGap = 5×Dit` (300 ms at 20 WPM) before the first auto-repeat, which is below `charBoundary` (6×Dit = 360 ms). USB paddle press durations measured at 30–100 ms via `make timing`. The extra elements appear to come from auto-repeat firing too early or from the IambicAdapter state machine allowing a repeat after a quick press+release cycle. The `make timing` tool is the right instrument for further diagnosis — log raw `KeyEvent` timestamps alongside `MorseInput` output while keying `f`.
**Depends on:** `IambicAdapter` + `CGEventTap` pipeline (current code).



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
