package main

import (
	"bytes"
	"math"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
)

const (
	sampleRate   = 44100
	channelCount = 1
)

// AudioPlayer wraps an oto context for synchronous Morse audio playback.
type AudioPlayer struct {
	ctx    *oto.Context
	freq   float64
	volume float64

	// active holds references to in-flight PlayNoWait players.
	// oto v3.4 auto-closes players when they are garbage collected,
	// which cuts off audio mid-play. Keeping references here prevents that.
	mu     sync.Mutex
	active []*oto.Player
}

// NewAudioPlayer initializes the audio subsystem with the given tone frequency
// (Hz) and volume (0.0–1.0). Blocks until the audio device is ready.
func NewAudioPlayer(freq, volume float64) (*AudioPlayer, error) {
	ctx, readyCh, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: channelCount,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		return nil, err
	}
	<-readyCh
	return &AudioPlayer{ctx: ctx, freq: freq, volume: volume}, nil
}

// Close is a no-op; oto.Context does not expose a Close method.
func (p *AudioPlayer) Close() {}

// Play writes PCM data to the audio device and blocks until playback finishes.
func (p *AudioPlayer) Play(data []byte) {
	if len(data) == 0 {
		return
	}
	player := p.ctx.NewPlayer(bytes.NewReader(data))
	player.Play()
	for player.IsPlaying() {
		time.Sleep(time.Millisecond)
	}
}

// PlayNoWait starts playback and returns immediately.
// It retains a reference to the player until playback completes so that
// Go's GC cannot collect (and thereby stop) an in-flight player.
// Must be called from a single goroutine.
func (p *AudioPlayer) PlayNoWait(data []byte) {
	if len(data) == 0 {
		return
	}
	// Drop finished players before adding the new one.
	p.mu.Lock()
	n := 0
	for _, pl := range p.active {
		if pl.IsPlaying() {
			p.active[n] = pl
			n++
		}
	}
	p.active = p.active[:n]
	p.mu.Unlock()

	pl := p.ctx.NewPlayer(bytes.NewReader(data))
	pl.Play()

	p.mu.Lock()
	p.active = append(p.active, pl)
	p.mu.Unlock()
}

// BuildAudio converts a sequence of Morse elements into a signed int16 LE
// PCM byte slice ready for playback.
func BuildAudio(elements []Element, timing Timing, freq, volume float64) []byte {
	var buf []byte
	for _, el := range elements {
		switch el.Kind {
		case KindDit:
			buf = append(buf, tone(freq, volume, timing.Dit)...)
		case KindDah:
			buf = append(buf, tone(freq, volume, timing.Dah)...)
		case KindToneGap:
			buf = append(buf, silence(timing.ToneGap)...)
		case KindCharGap:
			buf = append(buf, silence(timing.CharGap)...)
		case KindWordGap:
			buf = append(buf, silence(timing.WordGap)...)
		}
	}
	// Append a full word gap so consecutive entries have proper spacing
	// when played back-to-back. Also gives the last tone's release room to settle.
	buf = append(buf, silence(timing.WordGap)...)
	return buf
}

// tone generates a sine-wave burst with a short linear attack and release
// envelope (5 ms each) to prevent audible clicks.
func tone(freq, volume float64, dur time.Duration) []byte {
	n := int(dur.Seconds() * float64(sampleRate))
	if n <= 0 {
		return nil
	}

	attack := 5 * sampleRate / 1000 // 5 ms
	release := 5 * sampleRate / 1000
	if attack > n/2 {
		attack = n / 2
	}
	if release > n/2 {
		release = n / 2
	}

	buf := make([]byte, n*2)
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		v := math.Sin(2 * math.Pi * freq * t)

		env := 1.0
		switch {
		case i < attack:
			env = float64(i) / float64(attack)
		case i >= n-release:
			env = float64(n-i) / float64(release)
		}

		s := int16(v * env * volume * 32767)
		buf[i*2] = byte(s)
		buf[i*2+1] = byte(s >> 8)
	}
	return buf
}

// silence generates silent PCM samples for the given duration.
func silence(dur time.Duration) []byte {
	n := int(dur.Seconds() * float64(sampleRate))
	if n <= 0 {
		return nil
	}
	return make([]byte, n*2)
}
