//go:build android

// Package mobile connects the shared game to Android's Ebitengine view.
package mobile

import (
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
	enginemobile "github.com/hajimehoshi/ebiten/v2/mobile"
	"github.com/olivierh59500/timebandits/internal/game"
)

type host struct {
	mu       sync.Mutex
	game     *game.Game
	pause    bool
	back     bool
	started  bool
	savePath string
}

var gameHost = &host{}

func init() {
	ebiten.SetTPS(game.TicksPerSecond)
	ebiten.SetScreenFilterEnabled(false)
	enginemobile.SetGame(gameHost)
}

// Configure permits resource creation after Android has initialized the view.
func Configure(savePath string) {
	gameHost.mu.Lock()
	gameHost.started = true
	gameHost.savePath = savePath
	gameHost.mu.Unlock()
}

// Suspend queues a pause without mutating game state from Android's UI thread.
func Suspend() {
	gameHost.mu.Lock()
	gameHost.pause = true
	gameHost.mu.Unlock()
}

// Back queues Android's system back gesture.
func Back() {
	gameHost.mu.Lock()
	gameHost.back = true
	gameHost.mu.Unlock()
}

// Dummy retains an exported binding symbol for ebitenmobile.
func Dummy() {}

func (h *host) Update() error {
	h.mu.Lock()
	started, pause, back, savePath := h.started, h.pause, h.back, h.savePath
	h.pause, h.back = false, false
	h.mu.Unlock()
	if h.game == nil && started {
		var err error
		h.game, err = game.New(game.Options{Touch: true, SavePath: savePath})
		if err != nil {
			return err
		}
	}
	if h.game == nil {
		return nil
	}
	if pause {
		h.game.Pause()
	}
	if back {
		h.game.Back()
	}
	return h.game.Update()
}

func (h *host) Draw(screen *ebiten.Image) {
	if h.game != nil {
		h.game.Draw(screen)
	}
}

func (h *host) Layout(width, height int) (int, int) {
	if h.game != nil {
		return h.game.Layout(width, height)
	}
	return max(width, 1), max(height, 1)
}
