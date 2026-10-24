package game

import (
	"strings"
	"testing"

	"github.com/olivierh59500/timebandits/internal/original"
)

func TestCompletedAdventureShowsTheFullOriginalMessage(t *testing.T) {
	g, err := New(Options{Mute: true})
	if err != nil {
		t.Fatal(err)
	}
	g.start(1, false)
	for world := 1; world < original.WorldCount; world++ {
		g.worldPhases[world] = 0xff
		g.worldDone[world] = true
	}
	if err := g.loadWorld(0); err != nil {
		t.Fatal(err)
	}
	g.runWorldTrigger(0, 0, 12, 9)
	if g.dialog == nil || g.dialog.resume == 0 {
		t.Fatalf("completed journal did not open: %#v", g.dialog)
	}
	journal := g.dialog
	g.dialog = nil
	g.runScript(journal.resume, journal.player, 0)
	if g.dialog == nil {
		t.Fatal("completed journal did not show the ending")
	}
	for _, phrase := range []string{"BROKEN THE TIME", "NEW DAY BRINGS", "BILL DUNLEVY & HARRY LAFNEAR"} {
		if !strings.Contains(g.dialog.text, phrase) {
			t.Fatalf("ending omitted %q: %q", phrase, g.dialog.text)
		}
	}
	positions := map[int]bool{}
	for _, glyph := range g.dialog.glyphs {
		positions[glyph.y] = true
		if glyph.y < 0 || glyph.y+10 > ScreenHeight {
			t.Fatalf("ending glyph %q is outside the screen at %d", glyph.character, glyph.y)
		}
	}
	for _, y := range []int{18, 108, 174} {
		if !positions[y] {
			t.Fatalf("ending omitted original text row %d", y)
		}
	}
}
