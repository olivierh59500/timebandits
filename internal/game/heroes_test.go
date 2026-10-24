package game

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHeroTablesKeepTheSixHighestScores(t *testing.T) {
	heroes := defaultTimeLords()
	if heroes[0].Name != "CRACKED BY A-HA" || heroes[0].Score != 900000 || heroes[0].WorldJournal[1] != 15 || !heroes[0].WorldVisited[16] {
		t.Fatalf("original first Timelord = %+v", heroes[0])
	}
	if !insertHero(&heroes, heroRecord{Name: "NEW HERO", Score: 12340, Rating: 5}) {
		t.Fatal("qualifying hero was rejected")
	}
	if heroes[1].Name != "NEW HERO" || heroes[1].Score != 12340 {
		t.Fatalf("inserted hero = %+v", heroes[1])
	}
	if insertHero(&heroes, heroRecord{Name: "ZERO", Rating: 5}) {
		t.Fatal("zero score entered a full table")
	}
}

func TestFinishedAdventureRecordsAndPersistsHero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saves.json")
	g, err := New(Options{Mute: true, Players: 1, SavePath: path})
	if err != nil {
		t.Fatal(err)
	}
	g.players[0].score = 123
	g.players[0].scorePending = 4
	g.worldIndex, g.worldPhase = 3, 2
	g.worldVisited[3] = true
	g.worldJournal[3] = 6
	g.script.setWord(6, 0x005a)
	g.finishAdventure()
	if g.scene != heroNameScene || len(g.heroQueue) != 1 || g.heroQueue[0].record.Score != 1270 || g.heroQueue[0].record.Artifacts != 0x005a {
		t.Fatalf("finished adventure scene %d queue %+v", g.scene, g.heroQueue)
	}
	g.heroInput = []rune("Ada")
	g.acceptHeroName()
	if g.scene != heroesScene || g.timeLords[1].Name != "ADA" || g.todaysHeroes[0].Name != "ADA" {
		t.Fatalf("recorded heroes = Timelords %+v today %+v", g.timeLords, g.todaysHeroes)
	}

	reloaded, err := New(Options{Mute: true, SavePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.timeLords[1].Name != "ADA" || reloaded.timeLords[1].Score != 1270 || !reloaded.timeLords[1].WorldVisited[3] || reloaded.timeLords[1].WorldJournal[3] != 2 || reloaded.timeLords[1].Artifacts != 0x005a {
		t.Fatalf("persisted Timelord = %+v", reloaded.timeLords[1])
	}
	if reloaded.todaysHeroes[0].Name != "====================" {
		t.Fatalf("session heroes persisted unexpectedly: %+v", reloaded.todaysHeroes[0])
	}
}

func TestVersionTwoSaveMigratesOriginalTimelords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saves.json")
	if err := os.WriteFile(path, []byte("{\"version\":2,\"slots\":[null,null]}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	g, err := New(Options{Mute: true, SavePath: path})
	if err != nil {
		t.Fatal(err)
	}
	if g.timeLords[0].Name != "CRACKED BY A-HA" || g.timeLords[0].Score != 900000 {
		t.Fatalf("migrated Timelords = %+v", g.timeLords[0])
	}
}
