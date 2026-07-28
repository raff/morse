package main

import (
	"testing"
	"time"
)

// TestPlayAsync exercises the quiz-mode playback path. oto allows only one
// context per process, so all cases share a single AudioPlayer.
func TestPlayAsync(t *testing.T) {
	ap, err := NewAudioPlayer(700, 0.2)
	if err != nil {
		t.Skipf("no audio device: %v", err)
	}
	defer ap.Close()

	t.Run("done fires when the tone ends", func(t *testing.T) {
		data := tone(700, 0.2, 300*time.Millisecond)

		start := time.Now()
		done, stop := ap.PlayAsync(data)
		if d := time.Since(start); d > 20*time.Millisecond {
			t.Errorf("PlayAsync blocked for %v, want immediate return", d)
		}

		select {
		case <-done:
			elapsed := time.Since(start)
			t.Logf("done after %v (tone is 300ms)", elapsed)
			if elapsed < 200*time.Millisecond {
				t.Errorf("done fired after %v, too early for a 300ms tone", elapsed)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("done never closed")
		}
		stop() // safe after natural completion
		stop() // and idempotent
	})

	t.Run("stop cuts playback short", func(t *testing.T) {
		data := tone(700, 0.2, 2*time.Second)

		start := time.Now()
		done, stop := ap.PlayAsync(data)
		time.Sleep(100 * time.Millisecond)
		stop()

		select {
		case <-done:
			elapsed := time.Since(start)
			t.Logf("stopped after %v (tone is 2s)", elapsed)
			if elapsed > 500*time.Millisecond {
				t.Errorf("stop took %v to take effect", elapsed)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("done never closed after stop")
		}
	})

	t.Run("empty audio completes immediately", func(t *testing.T) {
		done, stop := ap.PlayAsync(nil)
		defer stop()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("empty PlayAsync did not close done")
		}
	})
}
