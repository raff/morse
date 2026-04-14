package main

import (
	"bytes"
	"testing"
	"time"
)

// TestAsyncAudioLatency measures whether the async channel approach is actually
// non-blocking and how much overhead oto's NewPlayer() adds per call.
func TestAsyncAudioLatency(t *testing.T) {
	ap, err := NewAudioPlayer(700, 0.3)
	if err != nil {
		t.Skip("no audio device:", err)
	}

	timing := NewTiming(20, 20)
	ditTone := tone(700, 0.3, timing.Dit)
	t.Logf("dit duration: %v  (%d bytes)", timing.Dit, len(ditTone))

	// --- 1. How long does NewPlayer() take? ---
	t.Log("--- NewPlayer() overhead ---")
	for i := 0; i < 3; i++ {
		start := time.Now()
		p := ap.ctx.NewPlayer(bytes.NewReader(ditTone))
		t.Logf("  NewPlayer %d: %v", i+1, time.Since(start))
		_ = p
	}

	// --- 2. How long does a synchronous Play() block? And what does each step cost? ---
	t.Log("--- Sync Play() breakdown ---")
	{
		t0 := time.Now()
		player := ap.ctx.NewPlayer(bytes.NewReader(ditTone))
		t1 := time.Now()
		player.Play()
		t2 := time.Now()
		for player.IsPlaying() {
			time.Sleep(time.Millisecond)
		}
		t3 := time.Now()
		t.Logf("  NewPlayer:   %v", t1.Sub(t0))
		t.Logf("  Play():      %v", t2.Sub(t1))
		t.Logf("  busy-wait:   %v  (audio drain)", t3.Sub(t2))
		t.Logf("  total:       %v  (expected ~%v)", t3.Sub(t0), timing.Dit)
	}

	// --- 3. PlayNoWait: does each call return immediately even with active players? ---
	t.Log("--- PlayNoWait (current approach) ---")
	var maxPlay time.Duration
	for i := 0; i < 8; i++ {
		start := time.Now()
		ap.PlayNoWait(ditTone)
		lat := time.Since(start)
		if lat > maxPlay {
			maxPlay = lat
		}
		t.Logf("  call %d: %v", i+1, lat)
		time.Sleep(15 * time.Millisecond)
	}
	t.Logf("max PlayNoWait latency: %v", maxPlay)
	if maxPlay > 5*time.Millisecond {
		t.Errorf("PlayNoWait is blocking: %v (want < 5ms)", maxPlay)
	}
	time.Sleep(300 * time.Millisecond) // let audio drain
}

// TestTimerAccuracyWithAudio checks whether busy-waiting audio goroutines
// cause Go's timer to fire late — which would make charBoundary feel sluggish.
func TestTimerAccuracyWithAudio(t *testing.T) {
	ap, err := NewAudioPlayer(700, 0.3)
	if err != nil {
		t.Skip("no audio device:", err)
	}

	timing := NewTiming(20, 20)
	ditTone := tone(700, 0.3, timing.Dit)
	charBoundary := timing.CharGap * 2 // 360 ms at 20 WPM

	measure := func(label string, withAudio bool) {
		if withAudio {
			// Start a single audio goroutine like sendWord does.
			playCh := make(chan []byte, 16)
			go func() {
				for data := range playCh {
					ap.Play(data)
				}
			}()
			defer close(playCh)
			// Feed it several tones so it's busy the whole time.
			for range 4 {
				playCh <- ditTone
			}
		}

		var maxJitter time.Duration
		for range 5 {
			timer := time.NewTimer(charBoundary)
			start := time.Now()
			<-timer.C
			jitter := time.Since(start) - charBoundary
			if jitter > maxJitter {
				maxJitter = jitter
			}
		}
		t.Logf("%s: max timer jitter = %v (want < 5ms)", label, maxJitter)
	}

	measure("without audio", false)
	time.Sleep(100 * time.Millisecond)
	measure("with    audio", true)
	time.Sleep(500 * time.Millisecond)
}

// TestMainLoopLatencyWithAudio checks whether audio goroutines delay the
// goroutine that reads keypresses — the core of the send-mode event loop.
func TestMainLoopLatencyWithAudio(t *testing.T) {
	ap, err := NewAudioPlayer(700, 0.3)
	if err != nil {
		t.Skip("no audio device:", err)
	}

	timing := NewTiming(20, 20)
	ditTone := tone(700, 0.3, timing.Dit)

	measure := func(label string, withAudio bool) {
		// Fake chars channel: pre-fill with 20 keypress bytes.
		chars := make(chan byte, 20)
		for i := 0; i < 20; i++ {
			chars <- '['
		}

		var maxLat time.Duration
		for range 20 {
			if withAudio {
				go ap.Play(ditTone) // fire a goroutine just like sendWord does
			}
			start := time.Now()
			<-chars // simulate reading a keypress
			lat := time.Since(start)
			if lat > maxLat {
				maxLat = lat
			}
			time.Sleep(15 * time.Millisecond)
		}
		t.Logf("%s: max channel-read latency = %v", label, maxLat)
	}

	measure("without audio goroutines", false)
	time.Sleep(200 * time.Millisecond) // let any goroutines settle
	measure("with    audio goroutines", true)
	time.Sleep(500 * time.Millisecond) // let audio goroutines finish
}
