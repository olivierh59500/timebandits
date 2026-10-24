package game

import (
	"path/filepath"
	"testing"

	"github.com/olivierh59500/timebandits/internal/original"
)

func TestJournalSaveSlotsRoundTripNativeState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saves.json")
	g, err := New(Options{Mute: true, Players: 1, SavePath: path})
	if err != nil {
		t.Fatal(err)
	}
	g.tick = 1234
	g.worldPhases[1] = 3
	g.worldJournal[1] = 2
	g.worldVisited[1] = true
	g.worldDone[2] = true
	g.players[0].score = 4321
	g.players[0].power = 9
	g.players[0].key = true
	g.pellets, g.powerTimer, g.powerCombo = 73, 64, 3
	g.bullets = []bullet{{x: 40, y: 44, dx: 8, owner: 0, direction: 1, life: 7, active: true}}
	g.script.setWord(6, 0x00a5)
	g.script.setWord(100, 321)
	g.setCell(5, 5, 0, 0x93)
	if err := g.saveSlot(0); err != nil {
		t.Fatal(err)
	}

	restored, err := New(Options{Mute: true, Players: 2, SavePath: path})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := restored.restoreSlot(0)
	if err != nil || !loaded {
		t.Fatalf("restore slot = %t, %v", loaded, err)
	}
	if restored.tick != 1234 || restored.players[0].score != 4321 || restored.players[0].power != 9 || !restored.players[0].key {
		t.Fatalf("restored player state = tick %d player %+v", restored.tick, restored.players[0])
	}
	if restored.worldPhases[1] != 3 || restored.worldJournal[1] != 2 || !restored.worldVisited[1] || !restored.worldDone[2] {
		t.Fatalf("restored world state = phases %v journal %v visited %v done %v", restored.worldPhases, restored.worldJournal, restored.worldVisited, restored.worldDone)
	}
	if restored.script.word(6) != 0x00a5 || restored.script.word(100) != 321 {
		t.Fatalf("restored script words = %x/%d", restored.script.word(6), restored.script.word(100))
	}
	if restored.pellets != 73 || restored.powerTimer != 64 || restored.powerCombo != 3 {
		t.Fatalf("restored collectible state = %d/%d/%d", restored.pellets, restored.powerTimer, restored.powerCombo)
	}
	if len(restored.bullets) != 1 || restored.bullets[0].life != 7 {
		t.Fatalf("restored projectile = %+v", restored.bullets)
	}
	if _, tile := restored.cell(5, 5); tile != 0x93 {
		t.Fatalf("restored map tile = %02x, want 93", tile)
	}
	loaded, err = restored.restoreSlot(1)
	if err != nil || loaded {
		t.Fatalf("empty slot = %t, %v", loaded, err)
	}
}

func TestJournalBufferSelectionUsesOriginalOneBasedNames(t *testing.T) {
	g := &Game{script: scriptRuntime{state: make([]byte, original.ScriptStateSize)}}
	g.script.setString(2, "2")
	if slot, ok := g.scriptSaveSlot(2); !ok || slot != 1 {
		t.Fatalf("buffer 2 = slot %d, valid %t", slot, ok)
	}
	g.script.setString(2, "X")
	if _, ok := g.scriptSaveSlot(2); ok {
		t.Fatal("invalid journal buffer was accepted")
	}
}

func TestTimegatesJournalScriptSavesAndLoadsBuffer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal-saves.json")
	newGame := func() *Game {
		g, err := New(Options{Mute: true, Players: 1, SavePath: path})
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	advance := func(g *Game, answer string) {
		t.Helper()
		if g.dialog == nil {
			t.Fatal("journal script did not open a dialog")
		}
		dialog := g.dialog
		if dialog.wantsInput {
			g.script.setString(dialog.variable, answer)
		}
		g.dialog = nil
		g.runScript(dialog.resume, dialog.player, 0)
	}

	saving := newGame()
	saving.players[0].score = 777
	saving.runWorldTrigger(0, 0, 12, 9)
	if !saving.journal || saving.dialog == nil {
		t.Fatal("Timegates trigger did not open the journal")
	}
	advance(saving, "S")
	advance(saving, "1")
	if saving.saveSlots[0] == nil {
		t.Fatal("journal did not populate save buffer 1")
	}

	loading := newGame()
	loading.players[0].score = 1
	loading.runWorldTrigger(0, 0, 12, 9)
	advance(loading, "L")
	advance(loading, "1")
	if loading.players[0].score != 777 || loading.dialog != nil || loading.journal {
		t.Fatalf("journal load restored score %d, dialog %#v, journal %t", loading.players[0].score, loading.dialog, loading.journal)
	}
}
