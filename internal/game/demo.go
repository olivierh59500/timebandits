package game

import (
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

type demoNode struct {
	x, y, firstDirection int
}

var demoDirectionVectors = [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

const demoProjectileRange = 11*8 + 8

func (g *Game) updateDemoPlayer() {
	p := &g.players[0]
	if !p.active {
		g.resetDemoPlayer()
		g.loadDemoWorld(g.demoTargetWorld)
		return
	}
	g.demoWorldTick++
	aligned := p.x%16 == 0 && p.y%16 == 0
	if aligned {
		x, y := p.x/16, p.y/16
		if index := y*g.worldWidth + x; index >= 0 && index < len(g.demoVisited) {
			g.demoVisited[index] = true
		}
		if g.demoHasGoal && x == g.demoGoalX && y == g.demoGoalY {
			g.demoHasGoal = false
			p.frame = 0
			return
		}
		flags, _ := g.cell(x, y)
		if flags>>5&3 == 2 {
			// Waiting one update lets chained portals carry the player without
			// leaving a slow projectile behind at each destination.
			p.frame = 0
			g.demoStall = 0
			return
		}
	}
	if !aligned {
		if escapeDirection, threatened := g.demoMidCellEscapeDirection(); threatened {
			if escapeDirection >= 0 {
				vector := demoDirectionVectors[escapeDirection]
				g.move(0, vector[0], vector[1])
			} else {
				p.frame = 0
			}
			g.finishDemoMovement()
			return
		}
	}

	direction, found := p.direction, false
	if aligned {
		if escapeDirection, emerging := g.demoEmergingEscapeDirection(); emerging {
			direction, found = escapeDirection, true
		} else if fireDirection, target := g.demoFireDirection(); target && g.demoCanFire() {
			p.direction = fireDirection
			p.frame = 0
			g.fire(0)
			g.demoStall = 0
			return
		} else if escapeDirection, threatened := g.demoEscapeDirection(); threatened {
			direction, found = escapeDirection, true
		} else {
			if g.demoHasGoal {
				direction, _, _, found = g.demoPathDirection(func(x, y int, _, _ byte) bool {
					return x == g.demoGoalX && y == g.demoGoalY
				})
				if !found {
					g.demoHasGoal = false
				}
			}
			if !g.demoHasGoal {
				direction, g.demoGoalX, g.demoGoalY, found = g.selectDemoGoal()
				g.demoHasGoal = found
			}
		}
	}
	if !found && aligned {
		direction = (p.direction + 1) & 3
	}
	vector := demoDirectionVectors[direction]
	if aligned && g.demoHasGoal && p.x/16+vector[0] == g.demoGoalX && p.y/16+vector[1] == g.demoGoalY {
		flags, _ := g.cell(g.demoGoalX, g.demoGoalY)
		if group := flags >> 5 & 3; group != 0 {
			index := g.demoGoalY*g.worldWidth + g.demoGoalX
			if index >= 0 && index < len(g.demoVisited) {
				g.demoVisited[index] = true
			}
			g.demoHasGoal = false
		}
	}
	shoot := g.demoShouldFire(direction)
	g.move(0, vector[0], vector[1])
	if shoot {
		g.fire(0)
	}
	g.finishDemoMovement()
}

func (g *Game) demoEmergingEscapeDirection() (int, bool) {
	p := &g.players[0]
	if g.worldIndex == 5 {
		return 0, false
	}
	var emerging []*entity
	for index := range g.entities {
		object := &g.entities[index]
		if object.active && object.materialize > 0 && object.materialize <= 8 &&
			(object.kind < 10 || object.kind >= 42) && object.flags&0x0200 == 0 &&
			max(abs(object.x-p.x), abs(object.y-p.y)) <= 3*16 {
			emerging = append(emerging, object)
		}
	}
	if len(emerging) == 0 {
		return 0, false
	}
	bestDirection, bestDistance := -1, -1
	for offset := range 4 {
		direction := (p.direction + offset) & 3
		vector := demoDirectionVectors[direction]
		if !g.demoCellPassable(p.x/16+vector[0], p.y/16+vector[1], direction) {
			continue
		}
		distance := 1 << 30
		for _, object := range emerging {
			objectX, objectY := g.demoPredictedEntityPosition(object, 4)
			playerX, playerY := p.x+vector[0]*16, p.y+vector[1]*16
			distance = min(distance, max(abs(objectX-playerX), abs(objectY-playerY)))
		}
		if distance > bestDistance {
			bestDirection, bestDistance = direction, distance
		}
	}
	return bestDirection, bestDirection >= 0
}

func (g *Game) finishDemoMovement() {
	p := &g.players[0]
	if p.x == g.demoLastX && p.y == g.demoLastY {
		g.demoStall++
	} else {
		g.demoLastX, g.demoLastY = p.x, p.y
		g.demoStall = 0
	}
	if g.demoWorldTick > 100*TicksPerSecond || g.demoStall > 10*TicksPerSecond {
		g.resetDemoPlayer()
		g.loadDemoWorld(g.demoTargetWorld)
	}
}

func (g *Game) resetDemoPlayer() {
	g.players[0] = initialPlayer(0, 0, 2, true)
	g.players[1] = player{}
	g.gameOver = 0
}

func (g *Game) demoCanFire() bool {
	p := &g.players[0]
	if !p.active || p.fireWait != 0 {
		return false
	}
	active := 0
	for index := range g.bullets {
		if g.bullets[index].owner == 0 {
			active++
		}
	}
	return active < 2
}

func (g *Game) demoFireDirection() (int, bool) {
	bestDirection, bestDistance := 0, demoProjectileRange+1
	for index := range g.entities {
		object := &g.entities[index]
		if !g.demoShootable(object) {
			continue
		}
		direction, distance, aligned := demoLineDirection(g.players[0].x, g.players[0].y, object.x, object.y)
		if !aligned || distance >= bestDistance || !g.demoShotPathClear(direction, distance) {
			continue
		}
		bestDirection, bestDistance = direction, distance
	}
	return bestDirection, bestDistance <= demoProjectileRange
}

func (g *Game) demoEscapeDirection() (int, bool) {
	p := &g.players[0]
	currentDanger := g.demoThreatDistance(p.x, p.y)
	if currentDanger > 2*16 {
		return 0, false
	}
	bestDirection, bestDistance := -1, currentDanger
	for offset := range 4 {
		direction := (p.direction + offset) & 3
		vector := demoDirectionVectors[direction]
		x, y := p.x/16+vector[0], p.y/16+vector[1]
		if !g.demoCellPassable(x, y, direction) {
			continue
		}
		distance := g.demoThreatDistance(x*16, y*16)
		if distance > bestDistance {
			bestDirection, bestDistance = direction, distance
		}
	}
	return bestDirection, bestDirection >= 0
}

func (g *Game) demoMidCellEscapeDirection() (int, bool) {
	p := &g.players[0]
	if g.worldIndex == 5 {
		return 0, false
	}
	currentSafety := g.demoRouteSafety(p.direction, 4)
	if currentSafety > 0 {
		return 0, false
	}
	bestDirection, bestSafety := p.direction, currentSafety
	if safety := g.demoRouteSafety(p.direction^2, 4); safety > bestSafety {
		bestDirection, bestSafety = p.direction^2, safety
	}
	if safety := g.demoRouteSafety(-1, 4); safety > bestSafety {
		bestDirection = -1
	}
	return bestDirection, true
}

func (g *Game) demoRouteSafety(direction, steps int) int {
	p := &g.players[0]
	vector := [2]int{}
	if direction >= 0 {
		vector = demoDirectionVectors[direction&3]
	}
	best := 1 << 30
	for index := range g.entities {
		object := &g.entities[index]
		if !g.demoDangerous(object) {
			continue
		}
		for step := 1; step <= steps; step++ {
			if object.materialize > step {
				continue
			}
			objectX, objectY := g.demoPredictedEntityPosition(object, step)
			playerX := p.x + vector[0]*step*4
			playerY := p.y + vector[1]*step*4
			clearance := max(abs(objectX-playerX), abs(objectY-playerY)) - demoCollisionRadius(object.kind)
			best = min(best, clearance)
		}
	}
	return best
}

func (g *Game) demoPredictedEntityPosition(object *entity, steps int) (int, int) {
	x, y := object.x, object.y
	vector := demoDirectionVectors[object.direction&3]
	for step := 0; step < steps; step++ {
		clock := uint32(1) << uint((g.tick+step)&31)
		if object.speed != 0 && object.movement&clock != 0 {
			x += vector[0] * object.speed * 4
			y += vector[1] * object.speed * 4
		}
	}
	return x, y
}

func demoCollisionRadius(kind int) int {
	if kind == 2 || kind == 6 || kind == 7 {
		return 4
	}
	return 8
}

func (g *Game) demoThreatDistance(x, y int) int {
	best := 1 << 30
	for index := range g.entities {
		object := &g.entities[index]
		if !g.demoThreat(object) {
			continue
		}
		best = min(best, abs(object.x-x)+abs(object.y-y))
	}
	return best
}

func (g *Game) demoDangerous(object *entity) bool {
	return object.active && object.materialize <= 2 && (object.kind < 10 || object.kind >= 42) && object.flags&0x0200 == 0
}

func (g *Game) demoThreat(object *entity) bool {
	return g.demoDangerous(object) && object.materialize == 0
}

func (g *Game) demoShootable(object *entity) bool {
	return g.demoThreat(object) && object.kind >= 42 && object.flags&0x0010 == 0
}

func demoLineDirection(fromX, fromY, toX, toY int) (int, int, bool) {
	dx, dy := toX-fromX, toY-fromY
	switch {
	case dy < 0 && abs(dx) <= 8:
		return 0, -dy, true
	case dx > 0 && abs(dy) <= 8:
		return 1, dx, true
	case dy > 0 && abs(dx) <= 8:
		return 2, dy, true
	case dx < 0 && abs(dy) <= 8:
		return 3, -dx, true
	default:
		return 0, 0, false
	}
}

func (g *Game) demoShotPathClear(direction, distance int) bool {
	p := &g.players[0]
	vector := demoDirectionVectors[direction]
	for step := 1; step <= (distance+15)/16; step++ {
		x, y := p.x/16+vector[0]*step, p.y/16+vector[1]*step
		flags, tile := g.cell(x, y)
		collision, _ := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
		if collision == 0xf5 {
			return false
		}
		if collision == 0x8e {
			return true
		}
		if collision < 0x92 || collision == 0xc9 || collision == 0xf4 {
			return false
		}
		if collision >= 0xec && collision <= 0xef && collision-0xec == byte(direction) {
			return false
		}
	}
	return true
}

func (g *Game) demoShouldFire(direction int) bool {
	p := &g.players[0]
	vector := demoDirectionVectors[direction]
	nextX, nextY := p.x/16+vector[0], p.y/16+vector[1]
	flags, tile := g.cell(nextX, nextY)
	collision, _ := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
	if collision == 0xf5 {
		return false
	}
	if collision == 0x8e {
		return true
	}

	for index := range g.entities {
		object := &g.entities[index]
		if !g.demoShootable(object) {
			continue
		}
		targetDirection, distance, aligned := demoLineDirection(p.x, p.y, object.x, object.y)
		if aligned && targetDirection == direction && distance <= demoProjectileRange && g.demoShotPathClear(direction, distance) {
			return true
		}
	}
	return false
}

func (g *Game) selectDemoGoal() (int, int, int, bool) {
	p := &g.players[0]
	if g.worldIndex == 0 {
		targets := make(map[[2]int]bool)
		for index := range g.entities {
			object := &g.entities[index]
			if object.active && object.kind-23 == g.demoTargetWorld {
				targets[[2]int{object.x / 16, object.y / 16}] = true
			}
		}
		return g.demoPathDirection(func(x, y int, _, _ byte) bool {
			return targets[[2]int{x, y}]
		})
	}
	if !p.key {
		if direction, x, y, found := g.demoPathDirection(func(_, _ int, flags, tile byte) bool {
			collision, _ := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
			return collision == 0xf5
		}); found {
			return direction, x, y, true
		}
	}
	if direction, x, y, found := g.demoPathDirection(func(x, y int, flags, _ byte) bool {
		index := y*g.worldWidth + x
		return flags>>5&3 == 1 && index >= 0 && index < len(g.demoVisited) && !g.demoVisited[index]
	}); found {
		return direction, x, y, true
	}
	if direction, x, y, found := g.demoPathDirection(func(_, _ int, flags, _ byte) bool {
		return flags>>5&3 == 3
	}); found {
		return direction, x, y, true
	}
	return g.demoPathDirection(func(x, y int, _, _ byte) bool {
		index := y*g.worldWidth + x
		return index >= 0 && index < len(g.demoVisited) && !g.demoVisited[index]
	})
}

func (g *Game) demoPathDirection(goal func(x, y int, flags, tile byte) bool) (int, int, int, bool) {
	p := &g.players[0]
	startX, startY := p.x/16, p.y/16
	if startX < 0 || startY < 0 || startX >= g.worldWidth || startY >= g.worldHeight {
		return 0, 0, 0, false
	}
	visited := make([]bool, g.worldWidth*g.worldHeight)
	visited[startY*g.worldWidth+startX] = true
	queue := make([]demoNode, 1, len(visited))
	queue[0] = demoNode{x: startX, y: startY, firstDirection: -1}
	for head := 0; head < len(queue); head++ {
		node := queue[head]
		flags, tile := g.cell(node.x, node.y)
		if node.firstDirection >= 0 && goal(node.x, node.y, flags, tile) {
			return node.firstDirection, node.x, node.y, true
		}
		for offset := range 4 {
			direction := (p.direction + offset) & 3
			vector := demoDirectionVectors[direction]
			x, y := node.x+vector[0], node.y+vector[1]
			if !g.demoCellPassable(x, y, direction) {
				continue
			}
			cellFlags, _ := g.cell(x, y)
			if cellFlags>>5&3 == 2 {
				portal, err := g.resources.WorldPortal(g.worldIndex, g.worldPhase, int(cellFlags&0x1f))
				if err == nil {
					x, y = portal.X, portal.Y
				}
			}
			if x < 0 || y < 0 || x >= g.worldWidth || y >= g.worldHeight {
				continue
			}
			index := y*g.worldWidth + x
			if visited[index] {
				continue
			}
			visited[index] = true
			first := node.firstDirection
			if first < 0 {
				first = direction
			}
			queue = append(queue, demoNode{x: x, y: y, firstDirection: first})
		}
	}
	return 0, 0, 0, false
}

func (g *Game) demoCellPassable(x, y, direction int) bool {
	if x < 0 || y < 0 || x >= g.worldWidth || y >= g.worldHeight {
		return false
	}
	flags, tile := g.cell(x, y)
	collision, _ := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
	if collision == 0x8e || collision == 0xeb {
		return true
	}
	if collision < 0x92 || collision == 0xc9 || collision == 0xf4 && !g.players[0].key || collision == 0xf5 && g.players[0].key {
		return false
	}
	return collision < 0xec || collision > 0xef || collision-0xec != byte(direction)
}

func (g *Game) updateDemoDialog() {
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) || len(ebiten.AppendTouchIDs(nil)) > 0 {
		g.scene, g.demo, g.journal, g.dialog = titleScene, false, false, nil
		g.idle = 0
		return
	}
	g.continueDemoDialog()
}

func (g *Game) continueDemoDialog() {
	dialog := g.dialog
	if dialog == nil {
		return
	}
	if dialog.wantsInput {
		// Some original conversations repeat until the player carries a required
		// item. Leave a repeated prompt so the demonstration can keep touring.
		if dialog.resume != 0 && dialog.resume == g.demoDialogPC && g.tick == g.demoDialogAt {
			g.dialog = nil
			g.journal = false
			return
		}
		g.demoDialogPC, g.demoDialogAt = dialog.resume, g.tick
		answer := []rune(g.demoDialogAnswer(dialog))
		if len(answer) > dialog.maxLength {
			answer = answer[:dialog.maxLength]
		}
		dialog.input = answer
		g.script.setString(dialog.variable, string(answer))
	}
	resume, playerIndex := dialog.resume, dialog.player
	g.dialog = nil
	if resume != 0 {
		g.runScript(resume, playerIndex, 0)
	}
	if g.dialog == nil {
		g.journal = false
	}
}

func (g *Game) demoDialogAnswer(dialog *scriptDialog) string {
	text := strings.ToUpper(dialog.text)
	if g.journal {
		return "X"
	}
	switch g.worldIndex {
	case 1:
		if strings.Contains(text, "ON BUSINESS") {
			return "TALK"
		}
		if strings.Contains(text, "OFFER AS PAY") {
			return "WINE"
		}
		if strings.Contains(text, "GAME OF CARDS") || strings.Contains(text, "HOW ABOUT IT") {
			return "YES"
		}
		if strings.Contains(text, "WINE") {
			return "USE WINE"
		}
		return "INV"
	case 3:
		if dialog.maxLength <= 3 {
			return "TEN"
		}
		return "YES"
	case 7:
		if strings.Contains(text, "CHARM") {
			return "USE CHARM"
		}
		if dialog.maxLength >= 4 {
			return "000A"
		}
		return "INV"
	case 10:
		directions := [...]string{"NORTH", "EAST", "SOUTH", "WEST"}
		return directions[(g.demoWorldTick/TicksPerSecond)&3]
	case 12:
		if dialog.maxLength <= 3 {
			return "MAN"
		}
		return "YES"
	case 4, 14:
		return "A"
	}
	if dialog.maxLength == 1 {
		return "A"
	}
	return "YES"
}
