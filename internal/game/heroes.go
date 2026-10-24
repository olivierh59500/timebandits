package game

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/olivierh59500/timebandits/internal/original"
)

const heroCount = 6

var adventureIcons = [...]int{0, 2, 16, 5, 11, 4, 3, 1, 15, 9, 13, 10, 10, 14, 6, 12}

type heroRecord struct {
	Name         string                     `json:"name"`
	Score        int                        `json:"score"`
	Rating       int                        `json:"rating"`
	World        int                        `json:"world"`
	Phase        int                        `json:"phase"`
	WorldPhases  [original.WorldCount]uint8 `json:"world_phases"`
	WorldJournal [original.WorldCount]uint8 `json:"world_journal"`
	WorldVisited [original.WorldCount]bool  `json:"world_visited"`
	Artifacts    uint16                     `json:"artifacts"`
}

type pendingHero struct {
	record heroRecord
	player int
}

func defaultTimeLords() [heroCount]heroRecord {
	heroes := defaultTodaysHeroes()
	phases := [original.WorldCount]uint8{0, 15, 3, 15, 7, 15, 8, 15, 12, 6, 15, 8, 2, 11, 4, 5, 0}
	visited := [original.WorldCount]bool{}
	for world := range visited {
		visited[world] = true
	}
	heroes[0] = heroRecord{
		Name: "CRACKED BY A-HA", Score: 900000, Rating: 3,
		WorldPhases: phases, WorldJournal: phases, WorldVisited: visited,
	}
	return heroes
}

func defaultTodaysHeroes() [heroCount]heroRecord {
	var heroes [heroCount]heroRecord
	for index := range heroes {
		heroes[index] = heroRecord{Name: "====================", Rating: 4}
	}
	return heroes
}

func validateHeroes(heroes [heroCount]heroRecord) error {
	for index, hero := range heroes {
		validName := len(hero.Name) <= 20
		for _, character := range []byte(hero.Name) {
			validName = validName && character >= 32 && character < 127
		}
		if !validName || hero.Score < 0 || hero.Rating < 0 || hero.Rating >= original.RatingCount || hero.World < 0 || hero.World >= original.WorldCount || hero.Phase < 0 || hero.Phase >= original.PhaseCount {
			return fmt.Errorf("hero %d has invalid data", index+1)
		}
		if index > 0 && heroes[index-1].Score < hero.Score {
			return fmt.Errorf("hero table is not sorted")
		}
	}
	return nil
}

func heroQualifies(heroes [heroCount]heroRecord, score int) bool {
	return score > heroes[len(heroes)-1].Score
}

func insertHero(heroes *[heroCount]heroRecord, hero heroRecord) bool {
	position := len(heroes)
	for index := range heroes {
		if hero.Score > heroes[index].Score {
			position = index
			break
		}
	}
	if position == len(heroes) {
		return false
	}
	copy(heroes[position+1:], heroes[position:len(heroes)-1])
	heroes[position] = hero
	return true
}

func (g *Game) finishAdventure() {
	if g.demo {
		g.resetDemoPlayer()
		g.loadDemoWorld(g.demoTargetWorld)
		return
	}
	g.heroQueue = g.heroQueue[:0]
	for index := range g.players {
		record := g.captureHero(index)
		if heroQualifies(g.timeLords, record.Score) || heroQualifies(g.todaysHeroes, record.Score) {
			g.heroQueue = append(g.heroQueue, pendingHero{record: record, player: index})
		}
	}
	g.gameOver = 0
	if len(g.heroQueue) == 0 {
		g.scene, g.idle = titleScene, 0
		return
	}
	g.heroInput = g.heroInput[:0]
	g.scene = heroNameScene
}

func (g *Game) captureHero(playerIndex int) heroRecord {
	p := &g.players[playerIndex]
	rating := p.rating
	if p.ratingSamples != 0 {
		rating = g.resources.RatingForScore(int(p.ratingTotal / uint64(p.ratingSamples)))
	}
	record := heroRecord{
		Score: (p.score + p.scorePending) * 10, Rating: rating,
		World: g.worldIndex, Phase: g.worldPhase,
		WorldPhases: g.worldPhases, WorldJournal: g.worldJournal, WorldVisited: g.worldVisited,
		Artifacts: uint16(g.script.word(6 + playerIndex)),
	}
	if g.worldIndex > 0 {
		record.WorldVisited[g.worldIndex] = true
		record.WorldJournal[g.worldIndex] = uint8(g.worldPhase)
	}
	return record
}

func (g *Game) updateHeroName() {
	g.tick++
	for _, character := range ebiten.AppendInputChars(nil) {
		character = unicode.ToUpper(character)
		if len(g.heroInput) < 20 && (character == ' ' || character >= '0' && character <= '9' || character >= 'A' && character <= 'Z' || character == '-') {
			g.heroInput = append(g.heroInput, character)
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(g.heroInput) > 0 {
		g.heroInput = g.heroInput[:len(g.heroInput)-1]
	}
	accept := inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyEscape)
	if g.options.Touch {
		for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
			x, y := ebiten.TouchPosition(id)
			character, action := scriptTouchKey(x, y)
			if character != 0 && len(g.heroInput) < 20 {
				g.heroInput = append(g.heroInput, character)
			}
			if action < 0 && len(g.heroInput) > 0 {
				g.heroInput = g.heroInput[:len(g.heroInput)-1]
			} else if action > 0 {
				accept = true
			}
		}
	}
	if accept {
		g.acceptHeroName()
	}
}

func (g *Game) acceptHeroName() {
	if len(g.heroQueue) == 0 {
		g.scene = heroesScene
		return
	}
	name := strings.ToUpper(strings.TrimSpace(string(g.heroInput)))
	if name == "" {
		name = "ANONYMOUS"
	}
	pending := g.heroQueue[0]
	pending.record.Name = name
	insertHero(&g.timeLords, pending.record)
	insertHero(&g.todaysHeroes, pending.record)
	g.heroQueue = g.heroQueue[1:]
	g.heroInput = g.heroInput[:0]
	if len(g.heroQueue) != 0 {
		return
	}
	if err := g.persistSaveSlots(); err != nil {
		g.dialog = &scriptDialog{text: "TIMELORD SAVE FAILED."}
	}
	g.scene, g.idle = heroesScene, 0
}

func (g *Game) updateHeroes() {
	g.tick++
	keys := inpututil.AppendJustPressedKeys(nil)
	for _, key := range keys {
		if key >= ebiten.KeyF1 && key <= ebiten.KeyF6 {
			g.heroDetail = int(key - ebiten.KeyF1)
			g.scene = heroDetailScene
			return
		}
	}
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		if x >= TouchGameX && x < TouchRightX && y >= 12 && y < 88 {
			g.heroDetail = min(5, max(0, (y-12)/12))
			g.scene = heroDetailScene
			return
		}
		g.scene, g.idle = titleScene, 0
		return
	}
	if len(keys) > 0 {
		g.scene, g.idle = titleScene, 0
	}
}

func (g *Game) updateHeroDetail() {
	g.tick++
	keys := inpututil.AppendJustPressedKeys(nil)
	for _, key := range keys {
		if key >= ebiten.KeyF1 && key <= ebiten.KeyF6 {
			g.heroDetail = int(key - ebiten.KeyF1)
			return
		}
	}
	if len(keys) > 0 || len(inpututil.AppendJustPressedTouchIDs(nil)) > 0 {
		g.scene = heroesScene
	}
}

func (g *Game) drawHeroes(screen *ebiten.Image) {
	screen.Fill(color.Black)
	phase := g.tick % palettePhases
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(256, 12)
	screen.DrawImage(g.lordsLogo[phase], op)
	op = &ebiten.DrawImageOptions{}
	op.GeoM.Translate(0, 96)
	screen.DrawImage(g.heroesLogo[phase], op)
	colors := [...]color.NRGBA{
		{238, 238, 238, 255}, {238, 102, 0, 255}, {238, 204, 0, 255},
		{0, 238, 0, 255}, {0, 238, 238, 255}, {170, 0, 238, 255},
	}
	for index := range heroCount {
		g.drawHeroRow(screen, g.timeLords[index], 8, 16+index*12, 176, colors[index])
		g.drawHeroRow(screen, g.todaysHeroes[index], 96, 108+index*12, 264, colors[index])
	}
	if g.options.Touch {
		g.drawText(screen, "TAP A TIMELORD FOR DETAILS", 48, 180, color.NRGBA{0, 238, 238, 255})
		g.drawText(screen, "TAP BACK TO CONTINUE", 80, 192, color.NRGBA{238, 102, 0, 255})
	} else {
		g.drawText(screen, "PRESS F1 THRU F6 FOR TIMELORD DETAILS", 8, 180, color.NRGBA{0, 238, 238, 255})
		g.drawText(screen, "PRESS ANY KEY TO CONTINUE", 56, 192, color.NRGBA{238, 102, 0, 255})
	}
}

func (g *Game) drawHeroRow(screen *ebiten.Image, hero heroRecord, nameX, y, scoreX int, tint color.Color) {
	name := hero.Name
	if len(name) > 20 {
		name = name[:20]
	}
	g.drawText(screen, fmt.Sprintf("%-20s", name), nameX, y, tint)
	g.drawText(screen, fmt.Sprintf("%7d", hero.Score), scoreX, y, tint)
}

func (g *Game) drawHeroName(screen *ebiten.Image) {
	g.drawHeroes(screen)
	vector.DrawFilledRect(screen, 0, 0, 248, 30, color.Black, false)
	player := 1
	if len(g.heroQueue) != 0 {
		player = g.heroQueue[0].player + 1
	}
	g.drawText(screen, fmt.Sprintf("CONGRATULATIONS, PLAYER %s!", []string{"ONE", "TWO"}[player-1]), 0, 2, color.NRGBA{238, 238, 238, 255})
	g.drawText(screen, "YOUR NAME? "+string(g.heroInput)+"_", 0, 16, color.NRGBA{0, 238, 238, 255})
}

func (g *Game) drawHeroDetails(screen *ebiten.Image) {
	screen.Fill(color.Black)
	hero := g.timeLords[g.heroDetail]
	cyan := color.NRGBA{0, 238, 238, 255}
	white := color.NRGBA{238, 238, 238, 255}
	red := color.NRGBA{204, 0, 0, 255}
	yellow := color.NRGBA{238, 238, 0, 255}
	separator := color.NRGBA{170, 136, 0, 255}
	green := color.NRGBA{0, 136, 0, 255}
	g.drawText(screen, fmt.Sprintf("%d:", g.heroDetail+1), 0, 0, cyan)
	g.drawText(screen, hero.Name, 24, 0, white)
	g.drawText(screen, "SCORE:", 208, 0, cyan)
	g.drawText(screen, fmt.Sprintf("%7d", hero.Score), 264, 0, white)
	g.drawText(screen, "OVERALL MANNER:", 0, 12, cyan)
	profile, err := g.resources.RatingProfile(hero.Rating)
	if err == nil {
		g.drawText(screen, profile.Name, 144, 12, white)
	}
	g.drawText(screen, strings.Repeat("=", 40), 0, 25, green)
	g.drawHeroArtifacts(screen, hero, 30, red)
	g.drawText(screen, strings.Repeat("=", 17), 24, 54, separator)
	g.drawText(screen, strings.Repeat("=", 20), 0, 78, separator)
	g.drawText(screen, strings.Repeat("=", 20), 160, 78, separator)
	g.drawText(screen, strings.Repeat("=", 20), 0, 130, separator)
	g.drawText(screen, strings.Repeat("=", 20), 160, 130, separator)
	phase := g.tick % palettePhases
	slots := [...]struct {
		world, x, y int
	}{
		{1, 160, 30}, {3, 160, 46}, {5, 0, 61}, {15, 160, 62},
		{4, 0, 81}, {9, 160, 81}, {11, 0, 97}, {7, 160, 97},
		{6, 0, 113}, {14, 160, 113}, {2, 0, 132}, {13, 160, 132},
		{16, 0, 148}, {10, 160, 148}, {8, 0, 164}, {12, 160, 164},
	}
	names := [...]string{
		"GREYMOON", "BOMB FACTORY", "KING'S CROWN", "MAJOR HAZARD",
		"SHADOWLAND", "OMEGA COMPLEX", "EXCALIBUR", "HOTEL CALIF.",
		"DARKSIDE DARE", "CHEOPS' CURSE", "GRIDVILLE", "GUARDIAN",
		"SENTINEL", "WELKIN ISLAND", "ARENA", "GHOST TOWN",
	}
	for _, slot := range slots {
		world := slot.world
		g.drawTile(screen, g.heroIcons[phase], adventureIcons[world-1], slot.x, slot.y)
		if !hero.WorldVisited[world] {
			g.drawText(screen, strings.Repeat("=", 17), slot.x+24, slot.y+4, separator)
			continue
		}
		worldPhase := int(hero.WorldJournal[world])
		progress := string([]byte{byte('1' + worldPhase/4), byte('A' + worldPhase%4)})
		g.drawText(screen, progress, slot.x+24, slot.y+4, yellow)
		g.drawText(screen, names[world-1], slot.x+48, slot.y+4, red)
	}
	g.drawText(screen, strings.Repeat("=", 40), 0, 182, green)
	if g.options.Touch {
		g.drawText(screen, "TAP TO RETURN", 104, 190, red)
	} else {
		g.drawText(screen, "ANY KEY TO CONTINUE. OTHER LORDS: F1-F6.", 0, 190, red)
	}
}

func (g *Game) drawHeroArtifacts(screen *ebiten.Image, hero heroRecord, y int, tint color.Color) {
	phase := g.tick % palettePhases
	g.drawTile(screen, g.heroIcons[phase], 17, 0, y)
	if hero.Artifacts == 0 {
		g.drawText(screen, "(NO ARTIFACTS)", 24, y+4, tint)
		return
	}
	x := 32
	for artifact := range original.ArtifactCount {
		if hero.Artifacts&(1<<uint(artifact)) == 0 {
			continue
		}
		g.drawTile(screen, g.heroArtifacts[phase], artifact, x, y)
		x += 16
	}
}
