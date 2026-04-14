# TODOS

## High priority

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
