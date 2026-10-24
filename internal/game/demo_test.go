package game

import (
	"testing"

	"github.com/olivierh59500/timebandits/internal/original"
)

func TestDemoKeepsGoalWhileBreakingObstacle(t *testing.T) {
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
		mapData: mapData, demo: true, demoVisited: make([]bool, 16*16),
		demoHasGoal: true, demoGoalX: 7, demoGoalY: 5, random: 0x12b9b09f,
	}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.demoLastX, g.demoLastY = g.players[0].x, g.players[0].y
	g.setCell(6, 5, 0, 0x8e)
	for tick := 1; tick <= 8; tick++ {
		g.tick = tick
		if g.players[0].fireWait > 0 {
			g.players[0].fireWait--
		}
		g.updateDemoPlayer()
		g.updateBullets()
	}
	if _, tile := g.cell(6, 5); tile != 0x93 {
		t.Fatalf("demo obstacle tile = %02x, want opened tile 93", tile)
	}
	if g.players[0].x <= 5*16 || !g.demoHasGoal {
		t.Fatalf("demo abandoned goal after obstacle: x %d goal %t", g.players[0].x, g.demoHasGoal)
	}
}

func TestDemoDoesNotStopAtASecondKey(t *testing.T) {
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
		mapData: mapData, demo: true, demoVisited: make([]bool, 16*16),
		demoHasGoal: true, demoGoalX: 7, demoGoalY: 5, random: 0x12b9b09f,
	}
	for index := range g.demoVisited {
		g.demoVisited[index] = true
	}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.demoLastX, g.demoLastY = g.players[0].x, g.players[0].y
	g.setCell(6, 5, 0, 0xf5)
	g.setCell(7, 5, 0, 0xf5)
	for tick := 1; tick <= 8; tick++ {
		g.tick = tick
		if g.players[0].fireWait > 0 {
			g.players[0].fireWait--
		}
		g.updateDemoPlayer()
		g.updateBullets()
	}
	if !g.players[0].key {
		t.Fatal("demo player did not collect the first key")
	}
	if g.players[0].x == 6*16 && g.players[0].y == 5*16 && g.demoHasGoal {
		t.Fatal("demo player remained blocked by the second key")
	}
	if len(g.bullets) != 0 {
		t.Fatalf("demo fired %d bullets without a hostile target", len(g.bullets))
	}
}

func TestDemoFiresOnlyAtUsefulTargets(t *testing.T) {
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
		mapData: mapData,
	}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.setCell(6, 5, 0, 0xf5)
	g.entities = []entity{{x: 8 * 16, y: 5 * 16, kind: 42, active: true}}
	if g.demoShouldFire(1) {
		t.Fatal("demo fired through a key before collecting it")
	}
	g.setCell(6, 5, 0, 0x92)
	if !g.demoShouldFire(1) {
		t.Fatal("demo did not fire at an aligned hostile actor")
	}
	if direction, found := g.demoFireDirection(); !found || direction != 1 {
		t.Fatalf("demo combat direction = %d/%t, want right", direction, found)
	}
	g.entities[0].x = g.players[0].x + demoProjectileRange
	if direction, found := g.demoFireDirection(); !found || direction != 1 {
		t.Fatalf("demo did not use the full projectile range: direction %d, found %t", direction, found)
	}
	g.entities[0].x++
	if _, found := g.demoFireDirection(); found {
		t.Fatal("demo aimed beyond the projectile's physical range")
	}
	g.entities[0].x = 8 * 16
	g.entities[0].flags = 0x0200
	if g.demoShouldFire(1) {
		t.Fatal("demo fired at a non-hostile actor")
	}
	g.entities[0].active = false
	g.setCell(6, 5, 0, 0x8e)
	if !g.demoShouldFire(1) {
		t.Fatal("demo did not fire at a breakable obstacle")
	}
}

func TestDemoAvoidsAnEmergingEnemy(t *testing.T) {
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
		mapData: mapData,
	}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.entities = []entity{{x: 6 * 16, y: 5 * 16, kind: 42, materialize: 8, active: true}}
	direction, found := g.demoEmergingEscapeDirection()
	if !found || direction != 3 {
		t.Fatalf("demo emerging-enemy escape = %d/%t, want left", direction, found)
	}
}

func TestDemoReversesBetweenCellsBeforeContact(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, worldIndex: 1}
	g.players[0] = initialPlayer(5*16+4, 5*16, 1, true)
	g.entities = []entity{{
		x: 6*16 + 4, y: 5 * 16, kind: 42, direction: 3, speed: 1,
		movement: ^uint32(0), active: true,
	}}
	direction, found := g.demoMidCellEscapeDirection()
	if !found || direction != 3 {
		t.Fatalf("demo mid-cell escape = %d/%t, want left", direction, found)
	}
}

func TestDemoWaitsForAChainedPortal(t *testing.T) {
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
		mapData: mapData, demo: true, demoVisited: make([]bool, 16*16), demoStall: 7,
	}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.demoLastX, g.demoLastY = g.players[0].x, g.players[0].y
	g.setCell(5, 5, 2<<5|3, 0x92)
	g.updateDemoPlayer()
	if g.players[0].x != 5*16 || g.players[0].y != 5*16 || g.demoStall != 0 {
		t.Fatalf("demo left a chained portal at %d,%d with stall %d", g.players[0].x, g.players[0].y, g.demoStall)
	}
}

func TestDemoEvadesAnImmediateUnshootableThreat(t *testing.T) {
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
		mapData: mapData,
	}
	g.players[0] = initialPlayer(5*16, 5*16, 1, true)
	g.entities = []entity{{x: 6 * 16, y: 5 * 16, kind: 42, flags: 0x0010, active: true}}
	if _, found := g.demoFireDirection(); found {
		t.Fatal("demo tried to shoot an invulnerable threat")
	}
	direction, found := g.demoEscapeDirection()
	if !found || direction != 2 {
		t.Fatalf("demo escape direction = %d/%t, want down", direction, found)
	}
}

func TestDemoConsumesTeleportingScriptGoalBeforeWarp(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	g := &Game{resources: resources, random: 0x12b9b09f, demo: true}
	g.players[0] = initialPlayer(22*16, 32*16, 1, true)
	g.worldPhases[1] = 0
	if err := g.loadWorld(1); err != nil {
		t.Fatal(err)
	}
	g.players[0].x, g.players[0].y = 22*16, 32*16
	g.demoHasGoal, g.demoGoalX, g.demoGoalY = true, 23, 32
	g.updateDemoPlayer()
	index := 32*g.worldWidth + 23
	if g.demoHasGoal || !g.demoVisited[index] {
		t.Fatalf("teleporting trigger remained a goal: active %t visited %t", g.demoHasGoal, g.demoVisited[index])
	}
}

func TestDemoAnswersKnownRiddles(t *testing.T) {
	g := &Game{worldIndex: 1}
	if answer := g.demoDialogAnswer(&scriptDialog{text: "ARE YOU HERE ON BUSINESS?", maxLength: 10}); answer != "TALK" {
		t.Fatalf("Castle Greymoon opening answer = %q", answer)
	}
	if answer := g.demoDialogAnswer(&scriptDialog{text: "WHAT DO YOU OFFER AS PAY?", maxLength: 12}); answer != "WINE" {
		t.Fatalf("Castle Greymoon payment answer = %q", answer)
	}

	g.worldIndex = 3
	if answer := g.demoDialogAnswer(&scriptDialog{text: "THE SPIRITS ASK", maxLength: 3}); answer != "TEN" {
		t.Fatalf("King's Crown answer = %q", answer)
	}
	g.worldIndex = 12
	if answer := g.demoDialogAnswer(&scriptDialog{text: "WHAT WALKS", maxLength: 3}); answer != "MAN" {
		t.Fatalf("Guardian answer = %q", answer)
	}
	g.journal = true
	if answer := g.demoDialogAnswer(&scriptDialog{maxLength: 1}); answer != "X" {
		t.Fatalf("journal demo answer = %q", answer)
	}
}

func TestDemoLeavesPromptRepeatedWithoutAdvancingTime(t *testing.T) {
	g := &Game{
		tick:         73,
		journal:      true,
		demoDialogPC: 0x1234,
		demoDialogAt: 73,
		dialog:       &scriptDialog{resume: 0x1234, wantsInput: true},
	}
	g.continueDemoDialog()
	if g.dialog != nil || g.journal {
		t.Fatalf("repeated demo prompt remained active: dialog %v journal %t", g.dialog != nil, g.journal)
	}
}

func TestDemoMakesProgressAcrossWorldsAndDialogs(t *testing.T) {
	g, err := New(Options{Mute: true})
	if err != nil {
		t.Fatal(err)
	}
	g.start(1, true)
	seen := [original.WorldCount]bool{}
	for step := 0; step < 8000 && g.tick < 2500; step++ {
		seen[g.worldIndex] = true
		advanceDemoForTest(g)
	}
	worlds := 0
	for world := 1; world < original.WorldCount; world++ {
		if seen[world] {
			worlds++
		}
	}
	if g.tick < 2500 || worlds < 2 {
		t.Fatalf("demo reached tick %d across %d game worlds", g.tick, worlds)
	}
}

func TestDemoRunsEveryAdventureWithoutFreezing(t *testing.T) {
	for world := 1; world < original.WorldCount; world++ {
		g, err := New(Options{Mute: true})
		if err != nil {
			t.Fatal(err)
		}
		g.start(1, true)
		g.loadDemoWorld(world)
		startTick := g.tick
		longestStall, damage := 0, 0
		lastPower := g.players[0].power
		keyCount, collectedKey := 0, false
		for y := 0; y < g.worldHeight; y++ {
			for x := 0; x < g.worldWidth; x++ {
				flags, tile := g.cell(x, y)
				collision, _ := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
				if collision == 0xf5 {
					keyCount++
				}
			}
		}
		for step := 0; step < 6000 && g.tick-startTick < 1900; step++ {
			advanceDemoForTest(g)
			longestStall = max(longestStall, g.demoStall)
			if g.players[0].power < lastPower {
				damage += lastPower - g.players[0].power
			}
			lastPower = g.players[0].power
			collectedKey = collectedKey || g.players[0].key
		}
		if g.tick-startTick < 1900 {
			t.Fatalf("demo froze in world %d at tick %d", world, g.tick-startTick)
		}
		if longestStall > 32 {
			t.Fatalf("demo stopped moving in world %d for %d ticks", world, longestStall)
		}
		if !g.players[0].active || damage != 0 {
			t.Fatalf("demo combat in world %d left active=%t after %d hits", world, g.players[0].active, damage)
		}
		if keyCount > 0 && !collectedKey {
			t.Fatalf("demo did not collect any of the %d keys in world %d", keyCount, world)
		}
	}
}

func TestDemoResetsPlayerBetweenShowcases(t *testing.T) {
	g, err := New(Options{Mute: true})
	if err != nil {
		t.Fatal(err)
	}
	g.start(1, true)
	g.loadDemoWorld(1)
	g.players[0].active = false
	g.players[0].power = 0
	g.players[0].key = true
	g.demoTargetWorld = 2
	g.updateDemoPlayer()
	if g.worldIndex != 2 || !g.players[0].active || g.players[0].power != 14 || g.players[0].key {
		t.Fatalf("reset demo player = world %d player %+v", g.worldIndex, g.players[0])
	}
}

func TestDemoCompletesShadowlandAfterEveryPellet(t *testing.T) {
	g, err := New(Options{Mute: true})
	if err != nil {
		t.Fatal(err)
	}
	g.start(1, true)
	g.loadDemoWorld(5)
	step := 0
	for ; step < 2000 && g.worldIndex == 5; step++ {
		advanceDemoForTest(g)
	}
	if g.worldIndex != 0 || g.worldPhases[5] != 1 {
		t.Fatalf("Shadowland demo ended at step %d in world %d with phase %d", step, g.worldIndex, g.worldPhases[5])
	}
}

func advanceDemoForTest(g *Game) {
	if g.dialog != nil {
		g.continueDemoDialog()
		return
	}
	g.tick++
	for index := range g.players {
		if g.players[index].scorePending > 0 {
			g.players[index].scorePending--
			g.players[index].score++
		}
		if g.players[index].fireWait > 0 {
			g.players[index].fireWait--
		}
		if g.players[index].hurtWait > 0 {
			g.players[index].hurtWait--
		}
		if g.players[index].scriptWait > 0 {
			g.players[index].scriptWait--
		}
	}
	g.updateDemoPlayer()
	g.recordPlayerTrails()
	g.updateBullets()
	if !g.updateWorldCells() {
		g.updateScript()
		g.updateActors()
		g.updateEntityContacts()
		g.updateSpawns()
		g.updateEntities()
		g.updateRatings()
	}
	g.followPlayer()
}
