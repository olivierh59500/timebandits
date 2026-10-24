package game

import (
	"math/bits"
	"strings"
	"testing"

	"github.com/olivierh59500/timebandits/internal/original"
)

func TestTimegatesMovementUsesRecoveredCells(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, worldWidth: 26, worldHeight: 21, random: 0x12b9b09f}
	g.mapData = append([]byte(nil), resources.WorldMap(0, 0)...)
	g.players[0] = player{x: 5 * 16, y: 5 * 16, active: true}
	if !g.walkable(g.players[0].x, g.players[0].y) {
		t.Fatal("the original Timegates start is blocked")
	}
	before := g.players[0].x
	g.move(0, 1, 0)
	if g.players[0].x <= before || g.players[0].direction != 1 {
		t.Fatalf("right movement = x %d, direction %d", g.players[0].x, g.players[0].direction)
	}
	if g.walkable(-1, 0) || g.walkable(26*16, 0) {
		t.Fatal("movement escaped the original map bounds")
	}
}

func TestCardinalFacingMatchesOriginalAnimationTable(t *testing.T) {
	tests := []struct {
		dx, dy, want int
	}{{0, -1, 0}, {1, 0, 1}, {1, 1, 1}, {0, 1, 2}, {-1, 0, 3}, {-1, -1, 0}}
	for _, test := range tests {
		if got := direction(test.dx, test.dy); got != test.want {
			t.Fatalf("direction(%d,%d) = %d, want %d", test.dx, test.dy, got, test.want)
		}
	}
}

func TestBanditAnimationsUseOriginalNormalAndShadowTables(t *testing.T) {
	normal := player{direction: 1, frame: 5}
	advanceBanditAnimation(&normal)
	if normal.frame != 0 || banditAnimationFrame(&normal) != 9 {
		t.Fatalf("normal right animation reset = index %d graphic %d", normal.frame, banditAnimationFrame(&normal))
	}
	shadow := player{direction: 1, frame: 3, shadow: true}
	if frame := banditAnimationFrame(&shadow); frame != 9 {
		t.Fatalf("shadow right animation graphic = %d, want 9", frame)
	}
	shadow.direction, shadow.frame = 3, 4
	if frame := banditAnimationFrame(&shadow); frame != 12 {
		t.Fatalf("shadow left animation graphic = %d, want 12", frame)
	}
}

func TestWelkinIslandKeepsTheBanditStillFrame(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	mapData := make([]byte, original.MapCapacity)
	for offset := 1; offset < len(mapData); offset += 2 {
		mapData[offset] = 0x92
	}
	g := &Game{resources: resources, worldIndex: 14, worldWidth: 16, worldHeight: 16, mapData: mapData}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.players[0].frame = 4
	g.move(0, 1, 0)
	if g.players[0].frame != 0 {
		t.Fatalf("Welkin Island Bandit frame = %d, want 0", g.players[0].frame)
	}
}

func TestRecoveredCellRulesMatchOriginalCodes(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	newWorld := func(world, phase int) *Game {
		descriptor, err := resources.World(world, phase)
		if err != nil {
			t.Fatal(err)
		}
		return &Game{
			resources: resources, worldIndex: world, worldPhase: phase,
			worldWidth: descriptor.Width, worldHeight: descriptor.Height,
			mapData: append([]byte(nil), resources.WorldMap(world, phase)...),
			random:  0x12b9b09f,
		}
	}

	g := newWorld(1, 0)
	if !g.enterCell(0, 24, 45, 0) || !g.players[0].key || g.players[0].scorePending != 15 {
		t.Fatalf("key pickup = key %t, pending score %d", g.players[0].key, g.players[0].scorePending)
	}
	if !g.enterCell(0, 14, 41, 0) || g.players[0].key || g.players[0].scorePending != 40 {
		t.Fatalf("lock opening = key %t, pending score %d", g.players[0].key, g.players[0].scorePending)
	}
	locked := newWorld(1, 0)
	if locked.enterCell(0, 14, 41, 0) {
		t.Fatal("an unopened lock accepted a player without a key")
	}
	if g.enterCell(0, 10, 50, 1) || !g.enterCell(0, 10, 50, 0) {
		t.Fatal("the recovered one-way door accepted its blocked direction")
	}
	if !g.enterCell(0, 39, 40, 0) || !g.enterCell(0, 27, 52, 0) || g.players[0].scorePending != 70 {
		t.Fatalf("escalating treasure score = %d, want 70", g.players[0].scorePending)
	}

	dig := newWorld(9, 0)
	if dig.enterCell(0, 2, 1, 0) || dig.players[0].scorePending != 1 {
		t.Fatal("diggable wall did not consume one blocked movement")
	}
	_, tile := dig.cell(2, 1)
	if tile < 0x8f || tile > 0x91 {
		t.Fatalf("dug tile = %02x, want 8f..91", tile)
	}
}

func TestShadowlandCompletesAfterAll181Pellets(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = initialPlayer(0, 0, 2, true)
	g.worldPhases[5] = 0
	if err := g.loadWorld(5); err != nil {
		t.Fatal(err)
	}
	x, y := -1, -1
	for row := 0; row < g.worldHeight && x < 0; row++ {
		for column := 0; column < g.worldWidth; column++ {
			_, tile := g.cell(column, row)
			if tile == 0xe9 {
				x, y = column, row
				break
			}
		}
	}
	if x < 0 {
		t.Fatal("Shadowland has no ordinary pellet")
	}
	g.players[0].x, g.players[0].y = x*16, y*16
	g.pellets = 180
	if !g.updateWorldCells() || g.worldIndex != 0 || g.worldPhases[5] != 1 {
		t.Fatalf("last pellet left world=%d phase=%d count=%d", g.worldIndex, g.worldPhases[5], g.pellets)
	}
}

func TestShadowlandPowerPelletEatsEnemies(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = initialPlayer(0, 0, 2, true)
	if err := g.loadWorld(5); err != nil {
		t.Fatal(err)
	}
	x, y := -1, -1
	for row := 0; row < g.worldHeight && x < 0; row++ {
		for column := 0; column < g.worldWidth; column++ {
			_, tile := g.cell(column, row)
			if tile == 0xea {
				x, y = column, row
				break
			}
		}
	}
	if x < 0 {
		t.Fatal("Shadowland has no power pellet")
	}
	g.players[0].x, g.players[0].y = x*16, y*16
	if g.updateWorldCells() || g.powerTimer != 130 || g.pellets != 1 || g.players[0].scorePending != 5 {
		t.Fatalf("power pellet state = timer %d pellets %d score %d", g.powerTimer, g.pellets, g.players[0].scorePending)
	}
	g.entities = []entity{{x: g.players[0].x, y: g.players[0].y, kind: 42, active: true}}
	g.updateEntityContacts()
	if g.players[0].power != 14 || g.powerCombo != 1 || g.entities[0].kind != 15 || g.entities[0].life != 30 || g.players[0].scorePending != 15 {
		t.Fatalf("powered contact = player %+v combo %d entity %+v", g.players[0], g.powerCombo, g.entities[0])
	}
}

func TestShadowlandPowerPaletteUsesOriginalFinalBlink(t *testing.T) {
	g := &Game{}
	for _, test := range []struct {
		timer int
		want  bool
	}{{130, true}, {26, true}, {25, false}, {23, true}, {20, true}, {19, false}, {0, false}} {
		g.powerTimer = test.timer
		if got := g.powerPaletteActive(); got != test.want {
			t.Fatalf("power palette at timer %d = %t, want %t", test.timer, got, test.want)
		}
	}
}

func TestSpecialHazardDamagesEitherPlayer(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	mapData := make([]byte, original.MapCapacity)
	for offset := 1; offset < len(mapData); offset += 2 {
		mapData[offset] = 0x92
	}
	g := &Game{resources: resources, worldIndex: 1, worldWidth: 16, worldHeight: 16, mapData: mapData}
	g.players[1] = initialPlayer(6*16, 5*16, 2, true)
	g.setCell(6, 5, 0, 0xe7)
	g.updateWorldCells()
	if g.players[1].power != 13 {
		t.Fatalf("second player hazard power = %d, want 13", g.players[1].power)
	}
	if _, tile := g.cell(6, 5); tile != 0x9d {
		t.Fatalf("consumed hazard tile = %02x, want 9d", tile)
	}
}

func TestWorldTransitionsPreserveOriginalSpawnsAndPhase(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0].active = true
	if err := g.loadWorld(0); err != nil {
		t.Fatal(err)
	}
	if !g.activateExit(0, 1) {
		t.Fatal("Timegates exit did not enter Castle Greymoon")
	}
	if g.worldIndex != 1 || g.worldPhase != 0 || g.players[0].x != 7*16 || g.players[0].y != 31*16 {
		t.Fatalf("first Castle Greymoon spawn = world %d phase %d at %d,%d", g.worldIndex, g.worldPhase, g.players[0].x, g.players[0].y)
	}
	if !g.activateExit(0, 0) || g.worldIndex != 0 || g.worldPhases[1] != 1 {
		t.Fatalf("return to Timegates left world %d and phase %d", g.worldIndex, g.worldPhases[1])
	}
	if g.worldDone[1] || !g.worldVisited[1] || g.worldJournal[1] != 0 || g.script.word(9) != 1 {
		t.Fatalf("progression gate state = done %t visited %t journal %d script %d", g.worldDone[1], g.worldVisited[1], g.worldJournal[1], g.script.word(9))
	}
	if !g.activateExit(0, 1) {
		t.Fatal("second Timegates exit did not enter Castle Greymoon")
	}
	if g.worldPhase != 1 || g.players[0].x != 7*16 || g.players[0].y != (54-1-31)*16 {
		t.Fatalf("phase 1B spawn = phase %d at %d,%d", g.worldPhase, g.players[0].x, g.players[0].y)
	}
	g.players[0].x, g.players[0].y = 1*16, 16*16
	if transitioned := g.updateWorldCells(); transitioned || g.players[0].x != 37*16 || g.players[0].y != 15*16 {
		t.Fatalf("phase 1B portal destination = %d,%d", g.players[0].x, g.players[0].y)
	}
}

func TestWorldCompletionClosesGateAfterPhase4D(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0].active = true
	if err := g.loadWorld(0); err != nil {
		t.Fatal(err)
	}
	g.worldPhases[1] = original.PhaseCount - 1
	if !g.activateExit(0, 1) || !g.activateExit(0, 0) {
		t.Fatal("phase 4D transition failed")
	}
	if !g.worldDone[1] || g.worldPhases[1] != 0xff || g.worldJournal[1] != 15 || g.script.word(9) != -1 {
		t.Fatalf("completed gate state = done %t phase %02x journal %d script %d", g.worldDone[1], g.worldPhases[1], g.worldJournal[1], g.script.word(9))
	}
	closedMarker := false
	for index := range g.entities {
		if g.entities[index].kind == 23 && g.entities[index].x == 15*16 && g.entities[index].y == 14*16 {
			closedMarker = true
		}
		if g.entities[index].kind == 24 {
			g.players[0].x, g.players[0].y = g.entities[index].x, g.entities[index].y
		}
	}
	if !closedMarker {
		t.Fatal("phase 4D completion did not create the original closed-gate marker")
	}
	g.updateEntities()
	if g.worldIndex != 0 {
		t.Fatal("a completed Timegate reopened after phase 4D")
	}
}

func TestUnrecordedScriptExitAdvancesWithoutJournalEntry(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0].active = true
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	if !g.activateExitMode(0, 0, false) {
		t.Fatal("unrecorded script exit failed")
	}
	if g.worldIndex != 0 || g.worldPhases[1] != 1 || g.worldVisited[1] {
		t.Fatalf("unrecorded exit = world %d phase %d visited %t", g.worldIndex, g.worldPhases[1], g.worldVisited[1])
	}
}

func TestTimegatesStartupRecognizesCompleteAdventure(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0].active = true
	for world := 1; world < original.WorldCount; world++ {
		g.worldPhases[world] = 0xff
		g.worldDone[world] = true
	}
	if err := g.loadWorld(0); err != nil {
		t.Fatal(err)
	}
	closed := 0
	for index := range g.entities {
		if g.entities[index].kind == 23 {
			closed++
		}
	}
	if g.script.word(43) != 1 || closed != 17 {
		t.Fatalf("complete adventure marker = %d with %d closed-gate sprites", g.script.word(43), closed)
	}
}

func TestOriginalProjectileLimitAndCadence(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, worldWidth: 26, worldHeight: 21, random: 0x12b9b09f}
	g.mapData = append([]byte(nil), resources.WorldMap(0, 0)...)
	g.players[0] = player{x: 5 * 16, y: 5 * 16, direction: 1, active: true}
	g.fire(0)
	g.fire(0)
	if len(g.bullets) != 1 || g.bullets[0].dx != 8 {
		t.Fatalf("first projectile count/vector = %d/%d", len(g.bullets), g.bullets[0].dx)
	}
	g.players[0].fireWait = 0
	g.fire(0)
	g.players[0].fireWait = 0
	g.fire(0)
	if len(g.bullets) != 2 {
		t.Fatalf("projectile slot limit = %d, want 2", len(g.bullets))
	}
}

func TestPlayerProjectileUsesOriginalLifetime(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	mapData := make([]byte, original.MapCapacity)
	for offset := 1; offset < len(mapData); offset += 2 {
		mapData[offset] = 0x92
	}
	g := &Game{resources: resources, worldIndex: 1, worldWidth: 32, worldHeight: 16, mapData: mapData}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.fire(0)
	if len(g.bullets) != 1 || g.bullets[0].life != 12 {
		t.Fatalf("new projectile = %+v", g.bullets)
	}
	for range 11 {
		g.updateBullets()
	}
	if len(g.bullets) != 1 || g.bullets[0].x != 5*16+11*8 {
		t.Fatalf("projectile before expiry = %+v", g.bullets)
	}
	g.updateBullets()
	if len(g.bullets) != 0 {
		t.Fatalf("projectile survived its original lifetime: %+v", g.bullets)
	}
}

func TestAdaptiveRatingPenalizesStationaryPlay(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, worldIndex: 1}
	g.players[0] = initialPlayer(5*16, 5*16, 2, true)
	for tick := 1; tick <= 1000; tick++ {
		g.tick = tick
		g.recordPlayerTrails()
		g.updateRatings()
	}
	if g.players[0].rating <= 5 || g.players[0].killBonus >= 2 {
		t.Fatalf("stationary rating = %d with kill bonus %d", g.players[0].rating, g.players[0].killBonus)
	}
	if text := g.ratingText(&g.players[0]); strings.HasPrefix(text, "AVERAGE") {
		t.Fatalf("rating text did not change: %q", text)
	}
}

func TestDemoUsesTheOriginalGateSpawn(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0].active = true
	g.loadDemoWorld(1)
	if g.worldIndex != 1 || g.players[0].x != 7*16 || g.players[0].y != 31*16 {
		t.Fatalf("demo Castle Greymoon spawn = world %d at %d,%d", g.worldIndex, g.players[0].x, g.players[0].y)
	}
}

func TestDefeatedSecondPlayerBecomesAShadow(t *testing.T) {
	g := &Game{}
	g.players[0] = player{active: true, power: 14}
	g.players[1] = player{active: true, power: 0}
	g.damagePlayer(1)
	if !g.players[1].active || !g.players[1].shadow || g.players[1].power != 0 {
		t.Fatalf("defeated player = %+v", g.players[1])
	}
	g.award(1, 10)
	if g.players[1].scorePending != 0 {
		t.Fatal("a shadow received score")
	}
}

func TestSinglePlayerDefeatStartsGameOver(t *testing.T) {
	g := &Game{}
	g.players[0] = player{active: true, power: 0}
	g.damagePlayer(0)
	if g.players[0].active || g.players[0].shadow || g.gameOver != gameOverDurationTicks {
		t.Fatalf("single-player defeat = player %+v, game over %d", g.players[0], g.gameOver)
	}
}

func TestOriginalTransitionTimingAndText(t *testing.T) {
	if gameOverDelayTicks != 20 || gameOverMessageTicks != 91 || gameOverDurationTicks != 111 {
		t.Fatalf("game-over timing = delay %d, message %d, total %d", gameOverDelayTicks, gameOverMessageTicks, gameOverDurationTicks)
	}
	if pausePrompt != "PRESS SPACEBAR..." || len(pausePrompt) != 17 {
		t.Fatalf("pause prompt = %q", pausePrompt)
	}
	if gameOverPrompt != "YOU HAVE DIED...THIS ADVENTURE IS OVER." || len(gameOverPrompt) != 39 {
		t.Fatalf("game-over prompt = %q", gameOverPrompt)
	}
}

func TestGameplayCadenceMatchesMeasuredAmigaMovement(t *testing.T) {
	if TicksPerSecond != 20 || 4*TicksPerSecond != 80 {
		t.Fatalf("gameplay cadence = %d Hz and %d pixels/second", TicksPerSecond, 4*TicksPerSecond)
	}
}

func TestKingsCrownTriggerUsesRecoveredScriptText(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = player{active: true, power: 14}
	if err := g.loadWorld(3); err != nil {
		t.Fatal(err)
	}
	g.runWorldTrigger(0, 0, 26, 6)
	if g.dialog == nil || !strings.Contains(g.dialog.text, "SPIRITS OF ZARDOZ") {
		t.Fatalf("King's Crown trigger dialog = %#v", g.dialog)
	}
	for g.dialog != nil && !g.dialog.wantsInput {
		dialog := g.dialog
		g.dialog = nil
		g.runScript(dialog.resume, dialog.player, 0)
	}
	if g.dialog == nil || !g.dialog.wantsInput {
		t.Fatalf("King's Crown riddle did not request its answer: %#v", g.dialog)
	}
	dialog := g.dialog
	g.script.setString(dialog.variable, "TEN")
	g.dialog = nil
	g.runScript(dialog.resume, dialog.player, 0)
	if g.worldIndex != 3 || g.worldPhase != 11 || g.worldPhases[3] != 11 {
		t.Fatalf("TEN warp = world %d phase %d stored %d, want King's Crown 3D", g.worldIndex, g.worldPhase, g.worldPhases[3])
	}
}

func TestWorldScriptsPreserveUnchangedGlobalVariables(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = player{active: true, power: 14}
	if err := g.loadWorld(3); err != nil {
		t.Fatal(err)
	}
	g.script.setWord(100, 321)
	if err := g.loadWorld(4); err != nil {
		t.Fatal(err)
	}
	if got := g.script.word(100); got != 321 {
		t.Fatalf("global variable after world transition = %d, want 321", got)
	}
}

func TestWorldStartupRunsAfterPhaseCounters(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.worldPhases[2] = 5
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	if g.script.word(10) != 5 {
		t.Fatalf("Bomb Factory phase variable = %d, want 5", g.script.word(10))
	}
	if g.script.word(16) != 225 || g.script.word(17) != -46 {
		t.Fatalf("Castle Greymoon startup variables = %d/%d", g.script.word(16), g.script.word(17))
	}
}

func TestZeroLengthInputReturnsControlToGameplay(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = initialPlayer(24*16, 30*16, 2, true)
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	g.runWorldTrigger(0, 7, 24, 30)
	if g.dialog == nil || g.dialog.wantsInput || g.dialog.resume != 0 {
		t.Fatalf("zero-length input dialog = %#v", g.dialog)
	}
}

func TestScriptDirectionFollowsPhaseMirroring(t *testing.T) {
	tests := []struct{ phase, direction, want int }{
		{0, 0, 0}, {0, 1, 1},
		{1, 0, 2}, {1, 2, 0},
		{2, 1, 3}, {2, 3, 1},
		{3, 0, 2}, {3, 1, 3},
	}
	for _, test := range tests {
		g := &Game{worldPhase: test.phase}
		if got := g.scriptDirection(test.direction); got != test.want {
			t.Fatalf("phase %d direction %d = %d, want %d", test.phase, test.direction, got, test.want)
		}
	}
}

func TestScriptSpawnMaterializationModes(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, worldWidth: 64, worldHeight: 64, random: 0x12b9b09f}
	g.spawnScriptEntity(57, 2, 3, 1, false)
	g.spawnScriptEntity(57, 4, 5, 2, true)
	if len(g.entities) != 2 {
		t.Fatalf("script entity count = %d, want 2", len(g.entities))
	}
	if g.entities[0].spawnWait != 0 || g.entities[0].materialize != 0 {
		t.Fatalf("immediate script entity waits = %d/%d", g.entities[0].spawnWait, g.entities[0].materialize)
	}
	if g.entities[1].spawnWait != 21 || g.entities[1].materialize != 21 {
		t.Fatalf("materialized script entity waits = %d/%d", g.entities[1].spawnWait, g.entities[1].materialize)
	}
}

func TestTouchScriptKeyboardMapping(t *testing.T) {
	tests := []struct {
		x, y, action int
		character    rune
	}{{0, 100, 0, 'A'}, {240, 140, 0, 'Z'}, {240, 160, 0, '9'}, {470, 170, 0, ' '}, {100, 185, -1, 0}, {350, 185, 1, 0}}
	for _, test := range tests {
		character, action := scriptTouchKey(test.x, test.y)
		if character != test.character || action != test.action {
			t.Fatalf("touch key at %d,%d = %q/%d, want %q/%d", test.x, test.y, character, action, test.character, test.action)
		}
	}
}

func TestTouchPauseUsesNarrowRightPanel(t *testing.T) {
	if !touchPauseContains(TouchPauseX, TouchPauseY) || !touchPauseContains(TouchWidth-5, TouchPauseY+TouchPauseHeight-1) {
		t.Fatal("pause button does not cover its visible bounds")
	}
	if touchPauseContains(TouchRightX-1, TouchPauseY) || touchPauseContains(TouchPauseX, TouchPauseY+TouchPauseHeight) {
		t.Fatal("pause button accepts a touch outside its visible bounds")
	}
}

func TestTouchDirectionUsesStableCardinalSectors(t *testing.T) {
	tests := []struct {
		dx, dy   float64
		previous int
		want     int
	}{
		{0.1, 0.1, -1, -1},
		{0.8, 0.1, -1, 1},
		{-0.8, 0.1, -1, 3},
		{0.1, -0.8, -1, 0},
		{0.1, 0.8, -1, 2},
		{0.7, -0.65, 1, 1},
		{0.55, -0.8, 1, 0},
		{-0.7, -0.65, 1, 3},
	}
	for _, test := range tests {
		if got := cardinalTouchDirection(test.dx, test.dy, test.previous); got != test.want {
			t.Fatalf("touch direction %.2f,%.2f after %d = %d, want %d", test.dx, test.dy, test.previous, got, test.want)
		}
	}
	for direction, want := range [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}} {
		dx, dy := touchDirectionVector(direction)
		if dx != want[0] || dy != want[1] {
			t.Fatalf("touch vector %d = %d,%d, want %d,%d", direction, dx, dy, want[0], want[1])
		}
	}
}

func TestTouchTurnFinishesGridAlignment(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	mapData := make([]byte, original.MapCapacity)
	for offset := 1; offset < len(mapData); offset += 2 {
		mapData[offset] = 0x92
	}
	g := &Game{
		resources: resources, options: Options{Touch: true},
		worldIndex: 1, worldWidth: 16, worldHeight: 16, mapData: mapData,
	}
	g.players[0] = initialPlayer(5*16+12, 5*16, 1, true)
	g.moveTouch(0, 0, 1)
	if g.players[0].x != 6*16 || g.players[0].y != 5*16 || g.players[0].direction != 1 {
		t.Fatalf("touch alignment = %d,%d direction %d", g.players[0].x, g.players[0].y, g.players[0].direction)
	}
	g.moveTouch(0, 0, 1)
	if g.players[0].x != 6*16 || g.players[0].y != 5*16+4 || g.players[0].direction != 2 {
		t.Fatalf("touch turn = %d,%d direction %d", g.players[0].x, g.players[0].y, g.players[0].direction)
	}
}

func TestOriginalControllerDefaults(t *testing.T) {
	g, err := New(Options{Mute: true})
	if err != nil {
		t.Fatal(err)
	}
	if g.controllers != [2]int{2, 1} {
		t.Fatalf("controller defaults = %v, want joystick 2 and joystick 1", g.controllers)
	}
	for index := range 3 {
		g.controllers[0] = (g.controllers[0] + 1) % 3
		if g.controllers[0] != (index+3)%3 {
			t.Fatalf("controller cycle %d = %d", index, g.controllers[0])
		}
	}
	if controllerGamepadIndex(0) != -1 || controllerGamepadIndex(1) != 1 || controllerGamepadIndex(2) != 0 {
		t.Fatal("original joystick ports do not map to host gamepads")
	}
}

func TestShadowScriptStopsAfterChangingPlayers(t *testing.T) {
	resources := &original.Resources{WorldLogic: []byte{0x2b, 0x0d, 0x00, 0x7f, 0x17}}
	g := &Game{resources: resources, script: scriptRuntime{state: make([]byte, original.ScriptStateSize)}}
	g.players[0].active = true
	g.players[1].active = true
	g.runScript(original.WorldLogicBase, 0, 0)
	if !g.players[0].shadow || !g.players[1].shadow {
		t.Fatalf("shadow opcode left players unchanged: %t/%t", g.players[0].shadow, g.players[1].shadow)
	}
	if g.script.word(0) != 0 {
		t.Fatalf("shadow opcode continued into the next instruction: %d", g.script.word(0))
	}
}

func TestRestartScriptReturnsToTitle(t *testing.T) {
	resources := &original.Resources{WorldLogic: []byte{0x2e}}
	g := &Game{
		resources: resources, scene: playScene, demo: true, paused: true, gameOver: 12, idle: 7,
		script: scriptRuntime{state: make([]byte, original.ScriptStateSize)},
	}
	g.runScript(original.WorldLogicBase, 0, 0)
	if g.scene != titleScene || g.demo || g.paused || g.gameOver != 0 || g.idle != 0 {
		t.Fatalf("restart opcode left scene=%d demo=%t paused=%t game-over=%d idle=%d", g.scene, g.demo, g.paused, g.gameOver, g.idle)
	}
}

func TestTouchJournalUsesSingleTapChoices(t *testing.T) {
	tests := []struct {
		x      int
		prompt string
		want   rune
	}{
		{TouchGameX + 20, "ENTER S TO SAVE OR L TO LOAD", 'S'},
		{TouchRightX - 20, "ENTER S TO SAVE OR L TO LOAD", 'L'},
		{TouchGameX + 20, "SAVE TO WHICH BUFFER", '1'},
		{TouchRightX - 20, "LOAD FROM WHICH BUFFER", '2'},
	}
	for _, test := range tests {
		character, action := journalTouchKey(test.x, 180, test.prompt)
		if character != test.want || action != 1 {
			t.Fatalf("journal touch %q at %d = %q/%d, want %q/1", test.prompt, test.x, character, action, test.want)
		}
	}
}

func TestMaterializationUsesOriginalMaskSequence(t *testing.T) {
	tests := []struct{ remaining, mask int }{{21, 1}, {20, 1}, {19, 2}, {18, 1}, {2, 7}, {1, 0}, {0, 0}}
	for _, test := range tests {
		if got := materializeMask(test.remaining); got != test.mask {
			t.Fatalf("materialize mask with %d frames remaining = %d, want %d", test.remaining, got, test.mask)
		}
	}
}

func TestRecoveredActorsKeepInitialCooldownAndMaterialization(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.worldPhases[16] = 1
	if err := g.loadWorld(16); err != nil {
		t.Fatal(err)
	}
	if g.entities[2].spawnWait != 21 || g.entities[2].materialize != 21 {
		t.Fatalf("recovered materializing actor waits = %d/%d", g.entities[2].spawnWait, g.entities[2].materialize)
	}
}

func TestMaterializingActorMovesDuringDirectionCooldown(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	mapData := make([]byte, original.MapCapacity)
	for offset := 1; offset < len(mapData); offset += 2 {
		mapData[offset] = 0x92
	}
	g := &Game{
		resources: resources, worldIndex: 1, worldWidth: 16, worldHeight: 16,
		mapData: mapData, random: 0x12b9b09f,
		entities: []entity{{
			x: 5 * 16, y: 5 * 16, kind: 42, direction: 1, speed: 1,
			movement: 1, spawnWait: 2, materialize: 2, active: true,
		}},
	}
	g.updateActors()
	if g.entities[0].x != 5*16+4 || g.entities[0].spawnWait != 1 || g.entities[0].materialize != 1 {
		t.Fatalf("materializing movement = x %d waits %d/%d", g.entities[0].x, g.entities[0].spawnWait, g.entities[0].materialize)
	}
}

func TestDynamicActorsUseRecoveredSpawnTable(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = player{active: true, power: 14}
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	if len(g.entities) != original.EntityCount || g.entities[0].active || !g.entities[2].active {
		t.Fatalf("recovered actor slots were compacted: length %d slots 0/2 = %t/%t", len(g.entities), g.entities[0].active, g.entities[2].active)
	}
	before := 0
	for index := range g.entities {
		if g.entities[index].active {
			before++
		}
	}
	g.tick = 16
	g.updateSpawns()
	after := 0
	for index := range g.entities {
		if g.entities[index].active {
			after++
		}
	}
	if after != before+1 {
		t.Fatalf("spawned active entity count = %d, want %d", after, before+1)
	}
	spawned := g.entities[0]
	if spawned.x != 16*16 || spawned.y != 28*16 {
		t.Fatalf("first Castle Greymoon spawn = %d,%d", spawned.x, spawned.y)
	}
}

func TestFullSpawnPoolUsesOriginalOverflowSlot(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = player{active: true, power: 14}
	g.worldPhases[1] = 10
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	for index := range 5 {
		g.entities[index] = entity{kind: 999, active: true}
	}
	g.tick = 4
	g.updateSpawns()
	for index := range 4 {
		if g.entities[index].kind != 999 {
			t.Fatalf("spawn replaced pool slot %d", index)
		}
	}
	if !g.entities[4].active || g.entities[4].kind == 999 {
		t.Fatalf("overflow slot was not replaced: %+v", g.entities[4])
	}
}

func TestTransientActorsUseReservedOverflowSlot(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, entities: make([]entity, original.EntityCount)}
	for index := range 57 {
		g.entities[index] = entity{kind: 999, active: true}
	}
	g.spawnTransientEntityState(12, 5*16, 5*16, 0, 0, 6, 0, 0)
	for index := range 56 {
		if g.entities[index].kind != 999 {
			t.Fatalf("transient actor replaced pool slot %d", index)
		}
	}
	if !g.entities[56].active || g.entities[56].kind != 12 {
		t.Fatalf("transient overflow slot = %+v", g.entities[56])
	}
}

func TestProjectileUsesOriginalDeathAnimation(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f, tick: 5}
	g.players[0] = player{active: true, power: 14}
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	g.players[0].killBonus = 2
	g.entities = []entity{{x: 5 * 16, y: 5 * 16, kind: 57, speed: 1, vitality: 0, value: 20, firstFrame: -1, active: true}}
	g.bullets = []bullet{{x: 5 * 16, y: 5 * 16, owner: 0, active: true}}
	g.updateBullets()
	if len(g.bullets) != 0 || !g.entities[0].active || g.entities[0].kind != 10 || g.entities[0].life != 8 || g.entities[0].firstFrame != 20 {
		t.Fatalf("projectile impact = bullets %d, entity %+v", len(g.bullets), g.entities[0])
	}
	if g.players[0].scorePending != 2 {
		t.Fatalf("enemy reward = %d, want AVERAGE bonus 2", g.players[0].scorePending)
	}
	if g.entities[0].speed != 1 || g.entities[0].movement != bits.RotateLeft32(0x34924920, 5) {
		t.Fatalf("death movement = speed %d mask %08x", g.entities[0].speed, g.entities[0].movement)
	}
}

func TestResistantActorUsesOriginalHitCounter(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = player{active: true, power: 14}
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	g.players[0].killBonus = 2
	g.entities = []entity{{x: 5 * 16, y: 5 * 16, kind: 100, vitality: 4, flags: 0x100, active: true}}
	for hit := 0; hit < 5; hit++ {
		g.bullets = []bullet{{x: 5 * 16, y: 5 * 16, owner: 0, active: true}}
		g.updateBullets()
		if hit < 4 && g.entities[0].kind != 100 {
			t.Fatalf("resistant actor died after hit %d", hit+1)
		}
	}
	if g.entities[0].kind != 10 || g.players[0].scorePending != 2 {
		t.Fatalf("fifth hit = entity kind %d reward %d", g.entities[0].kind, g.players[0].scorePending)
	}
}

func TestFragmentingActorCreatesFourProjectiles(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = player{active: true, power: 14}
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	g.entities = []entity{{x: 5 * 16, y: 5 * 16, kind: 63, vitality: 0, flags: 2, active: true}}
	g.bullets = []bullet{{x: 5 * 16, y: 5 * 16, owner: 0, active: true}}
	g.updateBullets()
	fragments := 0
	for index := range g.entities {
		if g.entities[index].active && g.entities[index].kind == 2 {
			fragments++
		}
	}
	if fragments != 4 {
		t.Fatalf("fragment count = %d, want 4", fragments)
	}
}

func TestSplittingActorUsesRecoveredCountAndKind(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = initialPlayer(5*16, 5*16, 2, true)
	if err := g.loadWorld(7); err != nil {
		t.Fatal(err)
	}
	g.setCell(5, 5, 0, 0x92)
	g.entities = []entity{{
		x: 5 * 16, y: 5 * 16, kind: 109, flags: 0x80,
		successorKind: 42, splitCount: 2, active: true,
	}}
	g.bullets = []bullet{{x: 5 * 16, y: 5 * 16, owner: 0, direction: 1, active: true}}
	g.updateBullets()
	children := 0
	directions := []int{}
	for index := range g.entities {
		if g.entities[index].active && g.entities[index].kind == 42 {
			children++
			directions = append(directions, g.entities[index].direction)
		}
	}
	if children != 2 || len(directions) != 2 || directions[0] != 3 || directions[1] != 0 {
		t.Fatalf("split children = %d directions %v", children, directions)
	}
}

func TestFragmentsRequirePerpendicularGridAlignment(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, entities: make([]entity, original.EntityCount)}
	g.spawnFragments(5*16+4, 5*16, 0)
	directions := []int{}
	for index := range g.entities {
		if g.entities[index].active && g.entities[index].kind == 2 {
			directions = append(directions, g.entities[index].direction)
		}
	}
	if len(directions) != 2 || directions[0] != 3 || directions[1] != 1 {
		t.Fatalf("grid-aligned fragment directions = %v", directions)
	}
}

func TestNonHostileContactSetsOriginalScriptSignal(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, script: scriptRuntime{state: make([]byte, original.ScriptStateSize)}}
	g.players[0] = player{x: 5 * 16, y: 5 * 16, power: 14, active: true}
	g.entities = []entity{{x: 5 * 16, y: 5 * 16, kind: 111, flags: 0x0230, active: true}}
	g.updateEntityContacts()
	if g.script.word(8) != 1 || g.players[0].power != 14 || !g.entities[0].active {
		t.Fatalf("non-hostile contact = signal %d power %d active %t", g.script.word(8), g.players[0].power, g.entities[0].active)
	}
}

func TestFragmentingActorContactStillDamagesPlayer(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, script: scriptRuntime{state: make([]byte, original.ScriptStateSize)}}
	g.players[0] = player{x: 5 * 16, y: 5 * 16, power: 14, active: true}
	g.entities = []entity{{x: 5 * 16, y: 5 * 16, kind: 64, flags: 0x0003, active: true}}
	g.updateEntityContacts()
	if g.script.word(8) != 0 || g.players[0].power != 13 {
		t.Fatalf("fragmenting contact = signal %d power %d", g.script.word(8), g.players[0].power)
	}
}

func TestSegmentedActorsFollowPrecedingDirection(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	mapData := make([]byte, original.MapCapacity)
	for offset := 1; offset < len(mapData); offset += 2 {
		mapData[offset] = 0x92
	}
	g := &Game{
		resources: resources, worldIndex: 15, worldWidth: 16, worldHeight: 16,
		mapData: mapData, tick: 0,
	}
	g.players[0] = player{x: 4 * 16, y: 4 * 16, active: true}
	g.entities = []entity{
		{x: 4 * 16, y: 4 * 16, kind: 72, direction: 1, speed: 1, sequence: 11, flags: 0x400, movement: 1, active: true},
		{x: 3 * 16, y: 4 * 16, kind: 75, direction: 0, speed: 1, sequence: 12, flags: 0x800, movement: 1, active: true},
	}
	g.updateActors()
	if g.entities[0].x != 4*16+4 || g.entities[1].x != 3*16+4 || g.entities[1].direction != 1 {
		t.Fatalf("segmented movement = head %d/%d body %d/%d direction %d", g.entities[0].x, g.entities[0].y, g.entities[1].x, g.entities[1].y, g.entities[1].direction)
	}
}

func TestTrackingActorFollowsRecordedPlayerTrail(t *testing.T) {
	g := &Game{}
	g.players[0] = player{active: true, trailCursor: 1}
	g.players[0].trailPosition[0] = uint16(5<<8 | 5)
	g.players[0].trailPosition[1] = uint16(6<<8 | 5)
	object := entity{x: 5 * 16, y: 5 * 16, flags: 1}
	direction, ok := g.directionFromPlayerTrail(&object)
	if !ok || direction != 1 {
		t.Fatalf("trail direction = %d, found %t; want right", direction, ok)
	}
}

func TestActorTargetAlternatesWithOriginalClockBit(t *testing.T) {
	g := &Game{}
	g.players[0] = player{x: 8 * 16, y: 5 * 16, active: true}
	g.players[1] = player{x: 5 * 16, y: 2 * 16, active: true}
	object := entity{x: 5 * 16, y: 5 * 16}
	if direction, ok := g.directionToOriginalTarget(&object); !ok || direction != 0 {
		t.Fatalf("clock-clear target direction = %d, found %t; want up toward Bandit Two", direction, ok)
	}
	g.tick = 32
	if direction, ok := g.directionToOriginalTarget(&object); !ok || direction != 1 {
		t.Fatalf("clock-set target direction = %d, found %t; want right toward Bandit One", direction, ok)
	}
}

func TestSegmentHitPromotesNextBodyPart(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	g.players[0] = initialPlayer(5*16, 5*16, 2, true)
	if err := g.loadWorld(15); err != nil {
		t.Fatal(err)
	}
	g.setCell(5, 5, 0, 0x92)
	g.entities = []entity{
		{x: 5 * 16, y: 5 * 16, kind: 72, flags: 0x400, successorKind: 72, active: true},
		{x: 6 * 16, y: 5 * 16, kind: 75, flags: 0x800, successorKind: 72, active: true},
	}
	g.bullets = []bullet{{x: 5 * 16, y: 5 * 16, owner: 0, active: true}}
	g.updateBullets()
	if g.entities[0].kind != 10 || g.entities[1].kind != 72 || g.entities[1].flags != 0x420 || g.script.word(4) != 1 {
		t.Fatalf("segment promotion = head %+v next %+v count %d", g.entities[0], g.entities[1], g.script.word(4))
	}
}

func TestRangedActorFiresRecoveredProjectileKind(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	mapData := make([]byte, original.MapCapacity)
	for offset := 1; offset < len(mapData); offset += 2 {
		mapData[offset] = 0x92
	}
	g := &Game{
		resources: resources, worldIndex: 9, worldWidth: 16, worldHeight: 16,
		mapData: mapData, tick: 16, random: 1,
	}
	g.players[0] = player{x: 8 * 16, y: 5 * 16, active: true}
	g.entities = []entity{{
		x: 5 * 16, y: 5 * 16, kind: 91, direction: 0, speed: 1,
		sequence: 14, flags: 8, movement: 1 << 16, active: true,
	}}
	g.updateActors()
	projectiles := 0
	for index := range g.entities {
		if g.entities[index].active && g.entities[index].kind == 5 {
			projectiles++
			if g.entities[index].direction != 1 || g.entities[index].life != 10 {
				t.Fatalf("ranged projectile = %+v", g.entities[index])
			}
		}
	}
	if projectiles != 1 {
		t.Fatalf("ranged attack = %d projectiles", projectiles)
	}
}

func TestProjectileOpensOriginalBreakableCell(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f}
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	g.setCell(5, 5, 0, 0x8e)
	if g.projectilePassable(bullet{x: 5 * 16, y: 5 * 16}) {
		t.Fatal("projectile passed through a breakable cell")
	}
	if _, tile := g.cell(5, 5); tile != 0x93 {
		t.Fatalf("opened cell = %02x, want 93", tile)
	}
}

func TestEachPlayerCanOwnTwoProjectiles(t *testing.T) {
	g := &Game{}
	g.players[0] = player{active: true}
	g.players[1] = player{active: true}
	for playerIndex := range 2 {
		for range 3 {
			g.players[playerIndex].fireWait = 0
			g.fire(playerIndex)
		}
	}
	if len(g.bullets) != 4 {
		t.Fatalf("projectile count = %d, want two per player", len(g.bullets))
	}
}

func TestPlayerProjectileDamagesOtherBandit(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, worldIndex: 1, worldWidth: 64, worldHeight: 54}
	g.mapData = append([]byte(nil), resources.WorldMap(1, 0)...)
	g.players[0] = player{x: 4 * 16, y: 5 * 16, power: 14, active: true}
	g.players[1] = player{x: 5 * 16, y: 5 * 16, power: 14, active: true}
	g.bullets = []bullet{{x: 5*16 - 8, y: 5 * 16, dx: 8, owner: 0, direction: 1, active: true}}
	g.updateBullets()
	if g.players[1].power != 13 || len(g.bullets) != 0 {
		t.Fatalf("friendly projectile left player power at %d and count at %d", g.players[1].power, len(g.bullets))
	}
}

func TestReturningProjectileCanHitItsOwnerLateInLife(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	mapData := make([]byte, original.MapCapacity)
	for offset := 1; offset < len(mapData); offset += 2 {
		mapData[offset] = 0x92
	}
	g := &Game{resources: resources, worldIndex: 1, worldWidth: 16, worldHeight: 16, mapData: mapData}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.bullets = []bullet{{x: 5*16 + 8, y: 5 * 16, dx: -8, owner: 0, direction: 3, life: 8, active: true}}
	g.updateBullets()
	if g.players[0].power != 14 || len(g.bullets) != 1 {
		t.Fatal("a fresh returning projectile hit its owner")
	}
	g.bullets[0].x, g.bullets[0].life = 5*16+8, 7
	g.updateBullets()
	if g.players[0].power != 13 || len(g.bullets) != 0 {
		t.Fatalf("old returning projectile left power=%d bullets=%d", g.players[0].power, len(g.bullets))
	}
}
