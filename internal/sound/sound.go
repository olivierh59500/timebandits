// Package sound plays PCM effects reconstructed from the original Paula state.
package sound

import (
	"sync"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

const SampleRate = 44100

var (
	contextOnce sync.Once
	context     *audio.Context
)

// Bank owns the immutable original effects and currently playing voices.
type Bank struct {
	effects [10][]byte
	players []*audio.Player
}

// New creates a bank using Ebitengine's single process-wide audio context.
func New(effects [10][]byte) *Bank {
	contextOnce.Do(func() { context = audio.NewContext(SampleRate) })
	return &Bank{effects: effects}
}

// Play starts one original effect and permits effects to overlap.
func (b *Bank) Play(effect int) {
	if b == nil || effect < 0 || effect >= len(b.effects) || len(b.effects[effect]) == 0 {
		return
	}
	b.Update()
	player := context.NewPlayerFromBytes(b.effects[effect])
	player.Play()
	b.players = append(b.players, player)
}

// Update releases players after their effect has ended.
func (b *Bank) Update() {
	if b == nil {
		return
	}
	kept := b.players[:0]
	for _, player := range b.players {
		if player.IsPlaying() {
			kept = append(kept, player)
			continue
		}
		_ = player.Close()
	}
	b.players = kept
}
