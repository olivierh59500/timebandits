package game

import (
	"encoding/binary"
	"image/color"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/olivierh59500/timebandits/internal/original"
)

const numericVariableOffset = 0x24a4 - 0x2224

var scriptOperandLengths = [...]int{
	0, 0, 1, 0, 1, 1, 2, 2, 3, 3, 0, 0, 1, 2, 3, 2,
	2, 3, 2, 1, 0, 1, 1, 0, 2, 2, 1, 0, 2, 1, 2, 2,
	3, 3, 3, 3, 3, 4, 1, 0, 0, 1, 1, 0, 1, 2, 0,
}

type scriptRuntime struct {
	base, offsets int
	state         []byte
	message       strings.Builder
	display       string
	glyphs        []scriptGlyph
	displayGlyphs []scriptGlyph
	cursorX       int
	cursorY       int
	lineX         int
	style         int
	color         byte
}

type scriptDialog struct {
	text                string
	input               []rune
	maxLength, variable int
	resume, player      int
	inputX, inputY      int
	style               int
	color               byte
	glyphs              []scriptGlyph
	wantsInput          bool
}

type scriptGlyph struct {
	character rune
	x, y      int
	style     int
	color     byte
}

func (g *Game) loadScript(world, phase int) {
	base, offsets, err := g.resources.WorldScripts(world)
	if err != nil {
		g.script = scriptRuntime{}
		return
	}
	post := g.resources.WorldScriptState(world, phase)
	pre := g.resources.WorldScriptPreState(world, phase)
	state := g.script.state
	if len(state) != original.ScriptStateSize {
		state = append([]byte(nil), pre...)
	}
	for phaseWorld := 1; phaseWorld < original.WorldCount; phaseWorld++ {
		offset := numericVariableOffset + (8+phaseWorld)*2
		binary.BigEndian.PutUint16(state[offset:], uint16(int16(int8(g.worldPhases[phaseWorld]))))
	}
	for index := range state {
		if pre[index] != post[index] {
			state[index] = post[index]
		}
	}
	g.script = scriptRuntime{base: base, offsets: offsets, state: state, color: 0x25}
	g.dialog = nil
}

func (g *Game) updateScript() {
	if g.script.base == 0 || g.dialog != nil {
		return
	}
	g.script.setWord(0, 1)
	g.runScript(g.script.base, 0, 0)
}

func (g *Game) runWorldTrigger(playerIndex, trigger, x, y int) {
	if g.script.offsets == 0 || g.dialog != nil {
		return
	}
	g.script.setWord(0, int16(playerIndex+1))
	g.script.setWord((0x24d6-0x24a4)/2, int16(x))
	g.script.setWord((0x24d8-0x24a4)/2, int16(y))
	data := g.resources.WorldLogicAt(g.script.offsets + trigger*2)
	if len(data) < 2 {
		return
	}
	offset := int(int16(binary.BigEndian.Uint16(data)))
	g.runScript(g.script.base+offset, playerIndex, 0)
}

func (g *Game) runScript(pc, playerIndex, depth int) {
	if depth > 16 || pc == 0 || g.dialog != nil {
		return
	}
	if depth == 0 {
		defer func() {
			g.applyScriptPalette()
			g.flushScriptDisplay(playerIndex)
		}()
	}
	for operations := 0; operations < 2048; operations++ {
		data := g.resources.WorldLogicAt(pc)
		if len(data) == 0 {
			return
		}
		opcode := int(data[0])
		pc++
		switch opcode {
		case 0x00, 0x01:
			if text, glyphs := g.takeScriptMessage(); text != "" || len(glyphs) != 0 {
				g.script.display, g.script.displayGlyphs = text, glyphs
			}
		case 0x02:
			pc++
		case 0x03:
			text, next := g.readDisplayText(pc)
			_ = text
			pc = next
		case 0x04:
			if value, ok := g.logicByte(pc); ok {
				g.appendScriptText(g.script.stringValue(int(value)))
			}
			pc++
		case 0x05:
			if value, ok := g.logicByte(pc); ok {
				g.appendScriptText(string(byte(g.script.word(int(value)))&0x0f | '0'))
			}
			pc++
		case 0x06, 0x07:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			text, glyphs := g.takeScriptMessage()
			if text == "" && len(glyphs) == 0 {
				text, glyphs = g.script.display, g.script.displayGlyphs
			}
			g.script.display, g.script.displayGlyphs = "", nil
			maxLength := int(operands[0])
			resume := pc + 2
			if opcode == 0x06 && maxLength == 0 {
				resume = 0
			}
			g.dialog = &scriptDialog{
				text: text, maxLength: maxLength,
				variable: int(operands[1]), resume: resume, player: playerIndex,
				inputX: g.script.cursorX, inputY: g.script.cursorY,
				style: g.script.style, color: g.script.color, glyphs: glyphs, wantsInput: maxLength > 0,
			}
			return
		case 0x08:
			operands := g.logicBytes(pc, 3)
			if len(operands) < 3 {
				return
			}
			pc += 3
			left, right := g.script.word(int(operands[0])), g.script.word(int(operands[2]))
			condition := left == right
			switch operands[1] {
			case 1:
				condition = left != right
			case 2:
				condition = left < right
			case 3:
				condition = left > right
			case 4:
				condition = uint16(left)&uint16(right) != 0
			}
			if !condition {
				pc = g.skipScriptBlock(pc, 0)
			}
		case 0x09:
			operands := g.logicBytes(pc, 3)
			if len(operands) < 3 {
				return
			}
			pc += 3
			equal := g.script.stringValue(int(operands[0])) == g.script.stringValue(int(operands[2]))
			if operands[1] == 0 && !equal || operands[1] != 0 && equal {
				pc = g.skipScriptBlock(pc, 0)
			}
		case 0x0a:
			// A block terminator is a no-op when its condition was true.
		case 0x0b, 0x17, 0x1b:
			return
		case 0x0c:
			variable, ok := g.logicByte(pc)
			if !ok {
				return
			}
			pc++
			value, next := g.readCString(pc)
			g.script.setString(int(variable), value)
			pc = next
		case 0x0d:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			g.script.setWord(int(operands[0]), int16(int8(operands[1])))
			pc += 2
		case 0x0e:
			operands := g.logicBytes(pc, 3)
			if len(operands) < 3 {
				return
			}
			g.script.setWord(int(operands[0]), int16(binary.BigEndian.Uint16(operands[1:])))
			pc += 3
		case 0x0f:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			g.script.setString(int(operands[0]), g.script.stringValue(int(operands[1])))
			pc += 2
		case 0x10:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			g.script.setWord(int(operands[0]), g.script.word(int(operands[1])))
			pc += 2
		case 0x11:
			operands := g.logicBytes(pc, 3)
			if len(operands) < 3 {
				return
			}
			offset := int(operands[0]) * 2
			if offset+2 <= len(g.paletteData) {
				copy(g.paletteData[offset:offset+2], operands[1:])
				g.paletteDirty = true
			}
			pc += 3
		case 0x12:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			if int(operands[0]) < len(g.worldPhases) {
				g.worldPhases[operands[0]] = operands[1]
			}
			pc += 2
		case 0x13:
			count, ok := g.logicByte(pc)
			if !ok {
				return
			}
			for range int(count) + 1 {
				g.damagePlayer(playerIndex ^ 1)
			}
			pc++
		case 0x14:
			playerIndex ^= 1
		case 0x15, 0x16:
			index, ok := g.logicByte(pc)
			if !ok {
				return
			}
			pc++
			target := g.scriptTarget(int(index))
			if opcode == 0x15 {
				pc = target
				continue
			}
			g.runScript(target, playerIndex, depth+1)
			if g.dialog != nil {
				return
			}
		case 0x18:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			g.scriptTeleport(playerIndex, int(operands[0]), int(operands[1]))
			pc += 2
		case 0x19:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			dx, dy := int(int8(operands[0])), int(int8(operands[1]))
			if g.worldPhase&2 != 0 {
				dx = -dx
			}
			if g.worldPhase&1 != 0 {
				dy = -dy
			}
			p := &g.players[playerIndex]
			g.setPlayerTile(playerIndex, p.x/16+dx, p.y/16+dy)
			pc += 2
		case 0x1a, 0x29:
			exitIndex, ok := g.logicByte(pc)
			if !ok {
				return
			}
			g.activateExitMode(playerIndex, int(exitIndex&0x1f), opcode == 0x1a)
			return
		case 0x1c:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			g.script.randomizeString(int(operands[0]), int(operands[1]), g)
			pc += 2
		case 0x1d:
			value, ok := g.logicByte(pc)
			if !ok {
				return
			}
			g.award(playerIndex, int(value))
			pc++
		case 0x1e, 0x1f:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			value := g.script.word(int(operands[1]))
			if opcode == 0x1e {
				g.script.setWord(int(operands[0]), g.script.word(int(operands[0]))+value)
			} else {
				g.script.setWord(int(operands[0]), g.script.word(int(operands[0]))-value)
			}
			pc += 2
		case 0x20:
			operands := g.logicBytes(pc, 3)
			if len(operands) < 3 {
				return
			}
			g.script.setStringByte(int(operands[0]), int(operands[1]), operands[2])
			pc += 3
		case 0x21:
			operands := g.logicBytes(pc, 3)
			if len(operands) < 3 {
				return
			}
			x, y := g.scriptCoordinates(int(operands[1]), int(operands[2]))
			flags, _ := g.cell(x, y)
			g.setCell(x, y, flags&0x7f, operands[0])
			pc += 3
		case 0x22, 0x23, 0x24, 0x25:
			length := scriptOperandLengths[opcode]
			operands := g.logicBytes(pc, length)
			if len(operands) < length {
				return
			}
			direction := int(g.nextRandom() & 3)
			if opcode == 0x25 {
				direction = g.scriptDirection(int(operands[3] & 3))
			}
			g.spawnScriptEntity(int(operands[0]), int(operands[1]), int(operands[2]), direction, opcode == 0x23 || opcode == 0x24)
			pc += length
		case 0x26:
			value, ok := g.logicByte(pc)
			if !ok {
				return
			}
			g.players[playerIndex].hurtWait += int(value)
			pc++
		case 0x27:
			g.players[playerIndex].scriptWait = 0
		case 0x28:
			g.journal = true
		case 0x2a:
			effect, ok := g.logicByte(pc)
			if !ok {
				return
			}
			g.playSound(int(effect))
			pc++
		case 0x2b:
			for index := range g.players {
				if g.players[index].active {
					g.players[index].shadow = true
				}
			}
			return
		case 0x2c:
			variable, ok := g.logicByte(pc)
			if !ok {
				return
			}
			pc++
			slot, valid := g.scriptSaveSlot(int(variable))
			if !valid {
				g.dialog = &scriptDialog{text: "INVALID SAVE BUFFER.", player: playerIndex}
				return
			}
			if err := g.saveSlot(slot); err != nil {
				g.dialog = &scriptDialog{text: "SAVE FAILED.", player: playerIndex}
				return
			}
		case 0x2d:
			operands := g.logicBytes(pc, 2)
			if len(operands) < 2 {
				return
			}
			pc += 2
			slot, valid := g.scriptSaveSlot(int(operands[0]))
			if !valid {
				g.script.setString(int(operands[1]), "B")
				continue
			}
			loaded, err := g.restoreSlot(slot)
			if err != nil || !loaded {
				g.script.setString(int(operands[1]), "B")
				continue
			}
			return
		case 0x2e:
			g.scene, g.demo, g.paused, g.gameOver, g.idle = titleScene, false, false, 0, 0
			g.dialog, g.journal = nil, false
			g.script.message.Reset()
			g.script.display, g.script.glyphs, g.script.displayGlyphs = "", nil, nil
			return
		default:
			return
		}
	}
}

func (g *Game) applyScriptPalette() {
	if !g.paletteDirty {
		return
	}
	g.paletteDirty = false
	if err := g.buildWorldAtlases(g.worldIndex, g.worldPhase); err != nil {
		g.err = err
		g.done = true
	}
}

func (g *Game) skipScriptBlock(pc, depth int) int {
	if depth > 16 {
		return pc
	}
	for operations := 0; operations < 2048; operations++ {
		opcodeByte, ok := g.logicByte(pc)
		if !ok {
			return pc
		}
		opcode := int(opcodeByte)
		pc++
		if opcode == 0x0a {
			return pc
		}
		if opcode >= len(scriptOperandLengths) {
			return pc
		}
		pc += scriptOperandLengths[opcode]
		switch opcode {
		case 0x03:
			pc = g.skipDisplayText(pc)
		case 0x0c:
			_, pc = g.readCString(pc)
		case 0x08, 0x09:
			pc = g.skipScriptBlock(pc, depth+1)
		}
	}
	return pc
}

func (g *Game) readDisplayText(pc int) (string, int) {
	var result strings.Builder
	for count := 0; count < 4096; count++ {
		value, ok := g.logicByte(pc)
		if !ok {
			return result.String(), pc
		}
		pc++
		if value == 0 {
			return result.String(), pc
		}
		switch value {
		case 1, 9:
			result.WriteByte('\n')
			g.scriptNewline()
		case 2:
			if next, ok := g.logicByte(pc); ok {
				g.script.color = next
			}
			pc++
		case 3:
			coordinates := g.logicBytes(pc, 2)
			if len(coordinates) == 2 {
				if result.Len() > 0 && !strings.HasSuffix(result.String(), "\n") {
					result.WriteByte('\n')
				}
				g.scriptSectionBreak()
				g.script.cursorX, g.script.cursorY = int(coordinates[0]), int(coordinates[1])
				g.script.lineX = g.script.cursorX
			}
			pc += 2
		case 4, 5, 6:
			g.script.style = int(value - 4)
		case 7:
			pc++
		case 8:
			// The original renderer pads to column 40. The following line
			// control carries the visible cursor to its next position.
		case 10:
			result.Reset()
			g.script.message.Reset()
			g.script.glyphs = g.script.glyphs[:0]
			g.script.cursorX, g.script.cursorY, g.script.lineX = 0, 0, 0
		default:
			if value >= 32 && value < 127 {
				result.WriteByte(value)
				g.appendScriptText(string(value))
			}
		}
	}
	return result.String(), pc
}

func (g *Game) skipDisplayText(pc int) int {
	for count := 0; count < 4096; count++ {
		value, ok := g.logicByte(pc)
		if !ok {
			return pc
		}
		pc++
		if value == 0 {
			return pc
		}
		if value == 2 || value == 7 {
			pc++
		} else if value == 3 {
			pc += 2
		}
	}
	return pc
}

func (g *Game) appendScriptText(text string) {
	for _, character := range text {
		if character == '\n' {
			g.scriptNewline()
			continue
		}
		g.script.message.WriteRune(character)
		g.script.glyphs = append(g.script.glyphs, scriptGlyph{
			character: character, x: g.script.cursorX, y: g.script.cursorY,
			style: g.script.style, color: g.script.color,
		})
		g.script.cursorX++
	}
}

func (g *Game) scriptNewline() {
	if g.script.message.Len() == 0 || !strings.HasSuffix(g.script.message.String(), "\n") {
		g.script.message.WriteByte('\n')
	}
	g.script.cursorX = g.script.lineX
	g.script.cursorY += 10
}

func (g *Game) scriptSectionBreak() {
	if g.script.message.Len() > 0 && !strings.HasSuffix(g.script.message.String(), "\n") {
		g.script.message.WriteByte('\n')
	}
}

func (g *Game) readCString(pc int) (string, int) {
	var result []byte
	for len(result) < 255 {
		value, ok := g.logicByte(pc)
		if !ok {
			break
		}
		pc++
		if value == 0 {
			break
		}
		result = append(result, value)
	}
	return string(result), pc
}

func (g *Game) scriptTarget(index int) int {
	if g.script.offsets == 0 {
		return 0
	}
	data := g.resources.WorldLogicAt(g.script.offsets + index*2)
	if len(data) < 2 {
		return 0
	}
	return g.script.base + int(int16(binary.BigEndian.Uint16(data)))
}

func (g *Game) logicByte(address int) (byte, bool) {
	data := g.resources.WorldLogicAt(address)
	if len(data) == 0 {
		return 0, false
	}
	return data[0], true
}

func (g *Game) logicBytes(address, length int) []byte {
	data := g.resources.WorldLogicAt(address)
	if len(data) < length {
		return nil
	}
	return data[:length]
}

func (g *Game) flushScriptDisplay(player int) {
	if g.dialog != nil {
		return
	}
	text, glyphs := g.takeScriptMessage()
	if text == "" && len(glyphs) == 0 {
		text, glyphs = g.script.display, g.script.displayGlyphs
	}
	g.script.display, g.script.displayGlyphs = "", nil
	if strings.TrimSpace(text) != "" || len(glyphs) != 0 {
		g.dialog = &scriptDialog{
			text: text, player: player, glyphs: glyphs,
			inputX: g.script.cursorX, inputY: g.script.cursorY,
			style: g.script.style, color: g.script.color,
		}
	}
}

func (g *Game) takeScriptMessage() (string, []scriptGlyph) {
	text := strings.TrimSpace(g.script.message.String())
	g.script.message.Reset()
	glyphs := append([]scriptGlyph(nil), g.script.glyphs...)
	g.script.glyphs = g.script.glyphs[:0]
	return text, glyphs
}

func (g *Game) updateDialog() {
	dialog := g.dialog
	if dialog == nil {
		return
	}
	dismiss := false
	if dialog.wantsInput {
		for _, character := range ebiten.AppendInputChars(nil) {
			character = unicode.ToUpper(character)
			if len(dialog.input) < dialog.maxLength && (character == ' ' || character >= '0' && character <= '9' || character >= 'A' && character <= 'Z') {
				dialog.input = append(dialog.input, character)
			}
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(dialog.input) > 0 {
			dialog.input = dialog.input[:len(dialog.input)-1]
		}
		if g.options.Touch {
			for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
				x, y := ebiten.TouchPosition(id)
				character, action := scriptTouchKey(x, y)
				if g.journal && dialog.maxLength == 1 {
					character, action = journalTouchKey(x, y, dialog.text)
				}
				if character != 0 && len(dialog.input) < dialog.maxLength {
					dialog.input = append(dialog.input, character)
				}
				if action == -1 && len(dialog.input) > 0 {
					dialog.input = dialog.input[:len(dialog.input)-1]
				} else if action == 1 {
					dismiss = true
				}
			}
		}
	}
	dismiss = dismiss || inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsKeyJustPressed(ebiten.KeyEscape)
	if !dialog.wantsInput {
		dismiss = dismiss || inpututil.IsKeyJustPressed(ebiten.KeySpace) || len(ebiten.AppendTouchIDs(nil)) > 0
	}
	if !dismiss {
		return
	}
	if dialog.wantsInput {
		g.script.setString(dialog.variable, string(dialog.input))
	}
	resume, player := dialog.resume, dialog.player
	g.dialog = nil
	if resume != 0 {
		g.runScript(resume, player, 0)
	}
	if g.dialog == nil {
		g.journal = false
	}
}

func (g *Game) drawScriptDialog(screen *ebiten.Image) {
	if g.dialog == nil {
		return
	}
	if g.journal && !strings.Contains(strings.ToUpper(g.dialog.text), "DEAR PLAYER") {
		g.drawJournal(screen)
		return
	}
	if len(g.dialog.glyphs) != 0 {
		g.drawPositionedScriptDialog(screen)
		return
	}
	dialogHeight, lastTextY, inputY := float32(180), 150, 164
	if g.options.Touch && g.dialog.wantsInput {
		dialogHeight, lastTextY, inputY = 100, 68, 84
	}
	vector.DrawFilledRect(screen, 0, 0, ScreenWidth, dialogHeight, color.Black, false)
	lines := wrapScriptText(g.dialog.text, 38)
	y := 8
	for _, line := range lines {
		g.drawText(screen, line, 8, y, color.NRGBA{238, 238, 238, 255})
		y += 11
		if y > lastTextY {
			break
		}
	}
	if g.dialog.wantsInput {
		g.drawText(screen, "> "+string(g.dialog.input)+"_", 8, inputY, color.NRGBA{0, 238, 238, 255})
	} else {
		g.drawText(screen, "ENTER TO CONTINUE", 88, 164, color.NRGBA{0, 238, 238, 255})
	}
}

func (g *Game) drawPositionedScriptDialog(screen *ebiten.Image) {
	dialog := g.dialog
	dialogHeight := float32(ScreenHeight)
	if g.options.Touch && dialog.wantsInput {
		dialogHeight = 100
	}
	vector.DrawFilledRect(screen, 0, 0, ScreenWidth, dialogHeight, color.Black, false)
	palette := g.currentWorldPalette()
	for _, glyph := range dialog.glyphs {
		if glyph.y < 0 || glyph.y+10 > int(dialogHeight) {
			continue
		}
		first := palette[glyph.color>>4]
		second := palette[glyph.color&0x0f]
		g.drawTextStyle(screen, string(glyph.character), glyph.x*8, glyph.y, glyph.style, first, second)
	}
	if !dialog.wantsInput {
		return
	}
	inputY := dialog.inputY
	if g.options.Touch {
		inputY = min(inputY, 84)
	}
	first := palette[dialog.color>>4]
	second := palette[dialog.color&0x0f]
	g.drawTextStyle(screen, string(dialog.input)+"_", dialog.inputX*8, inputY, dialog.style, first, second)
}

func (g *Game) drawJournal(screen *ebiten.Image) {
	screen.Fill(color.Black)
	g.drawText(screen, "TIMEGATES JOURNAL", 92, 2, color.NRGBA{0, 238, 238, 255})
	artifactMask := uint16(g.script.word(6 + g.dialog.player))
	artifactCount := 0
	for artifact := range original.ArtifactCount {
		if artifactMask&(1<<uint(artifact)) != 0 {
			artifactCount++
		}
	}
	if artifactCount == 0 {
		g.drawText(screen, "(NO ARTIFACTS)", 104, 17, color.NRGBA{238, 238, 0, 255})
	} else {
		x := (ScreenWidth - artifactCount*16) / 2
		phase := g.tick % palettePhases
		for artifact := range original.ArtifactCount {
			if artifactMask&(1<<uint(artifact)) == 0 {
				continue
			}
			g.drawTile(screen, g.artifacts[phase], artifact, x, 14)
			x += 16
		}
	}
	names := [...]string{
		"GREYMOON", "BOMB FACTORY", "KING'S CROWN", "MAJOR HAZARD",
		"SHADOWLAND", "OMEGA COMPLEX", "EXCALIBUR", "HOTEL CALIF.",
		"DARKSIDE DARE", "CHEOPS' CURSE", "GRIDVILLE", "GUARDIAN",
		"SENTINEL", "WELKIN ISL.", "ARENA", "GHOST TOWN",
	}
	phase := g.tick % palettePhases
	for index, name := range names {
		column, row := index/8, index%8
		x, y := column*160, 32+row*17
		g.drawTile(screen, g.icons[phase], adventureIcons[index], x, y)
		if len(name) > 12 {
			name = name[:12]
		}
		g.drawText(screen, name, x+18, y+3, color.NRGBA{238, 238, 238, 255})
		world := index + 1
		if !g.worldVisited[world] {
			continue
		}
		worldPhase := int(g.worldJournal[world])
		progress := string([]byte{byte('1' + worldPhase/4), byte('A' + worldPhase%4)})
		g.drawText(screen, progress, x+136, y+3, color.NRGBA{0, 238, 238, 255})
	}
	prompt := strings.Join(strings.Fields(g.dialog.text), " ")
	if strings.Contains(prompt, "TIMEGATES JOURNAL") || prompt == "" {
		prompt = "S SAVE / L LOAD"
	}
	if len(prompt) > 38 {
		prompt = prompt[:38]
	}
	if g.options.Touch && g.dialog.wantsInput && g.dialog.maxLength == 1 {
		left, right := "S  SAVE", "L  LOAD"
		if strings.Contains(strings.ToUpper(g.dialog.text), "BUFFER") {
			left, right = "1", "2"
		}
		buttonColor := color.NRGBA{34, 70, 72, 255}
		vector.DrawFilledRect(screen, 4, 171, 152, 27, buttonColor, false)
		vector.DrawFilledRect(screen, 164, 171, 152, 27, buttonColor, false)
		g.drawText(screen, left, 80-len(left)*4, 180, color.White)
		g.drawText(screen, right, 240-len(right)*4, 180, color.White)
		return
	}
	promptY, inputY := 174, 187
	if g.options.Touch && g.dialog.wantsInput {
		promptY, inputY = 74, 87
	}
	g.drawText(screen, prompt, 8, promptY, color.NRGBA{238, 238, 0, 255})
	if g.dialog.wantsInput {
		g.drawText(screen, "> "+string(g.dialog.input)+"_", 8, inputY, color.NRGBA{0, 238, 238, 255})
	} else {
		g.drawText(screen, "ENTER TO CONTINUE", 168, inputY, color.NRGBA{0, 238, 238, 255})
	}
}

func scriptTouchKey(x, y int) (rune, int) {
	const (
		keyboardY = 100
		actionY   = 180
		columns   = 10
		keyWidth  = TouchWidth / columns
		keyHeight = 20
	)
	if x < 0 || x >= TouchWidth || y < keyboardY || y >= ScreenHeight {
		return 0, 0
	}
	if y >= actionY {
		if x < TouchWidth/2 {
			return 0, -1
		}
		return 0, 1
	}
	column, row := x/keyWidth, (y-keyboardY)/keyHeight
	index := row*columns + column
	const keys = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if row == 3 && column >= 6 {
		return ' ', 0
	}
	if index < 0 || index >= len(keys) {
		return 0, 0
	}
	return rune(keys[index]), 0
}

func journalTouchKey(x, y int, prompt string) (rune, int) {
	if x < TouchGameX || x >= TouchRightX || y < 170 || y >= ScreenHeight {
		return 0, 0
	}
	left := x-TouchGameX < ScreenWidth/2
	if strings.Contains(strings.ToUpper(prompt), "BUFFER") {
		if left {
			return '1', 1
		}
		return '2', 1
	}
	if left {
		return 'S', 1
	}
	return 'L', 1
}

func (g *Game) drawScriptKeyboard(screen *ebiten.Image) {
	const (
		keys      = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
		keyboardY = 100
		actionY   = 180
		columns   = 10
		keyWidth  = TouchWidth / columns
		keyHeight = 20
	)
	vector.DrawFilledRect(screen, 0, keyboardY, TouchWidth, ScreenHeight-keyboardY, color.NRGBA{8, 18, 20, 255}, false)
	for index, character := range keys {
		x, y := (index%columns)*keyWidth, keyboardY+(index/columns)*keyHeight
		vector.StrokeRect(screen, float32(x+1), float32(y+1), keyWidth-2, keyHeight-2, 1, color.NRGBA{0, 136, 136, 255}, false)
		g.drawText(screen, string(character), x+(keyWidth-8)/2, y+5, color.White)
	}
	spaceX := 6 * keyWidth
	vector.StrokeRect(screen, float32(spaceX+1), keyboardY+3*keyHeight+1, 4*keyWidth-2, keyHeight-2, 1, color.NRGBA{0, 136, 136, 255}, false)
	g.drawText(screen, "SPACE", spaceX+2*keyWidth-20, keyboardY+3*keyHeight+5, color.White)
	vector.StrokeRect(screen, 1, actionY+1, TouchWidth/2-2, ScreenHeight-actionY-2, 1, color.NRGBA{204, 0, 0, 255}, false)
	g.drawText(screen, "BACK", TouchWidth/4-16, actionY+5, color.White)
	vector.StrokeRect(screen, TouchWidth/2+1, actionY+1, TouchWidth/2-2, ScreenHeight-actionY-2, 1, color.NRGBA{0, 136, 136, 255}, false)
	g.drawText(screen, "ENTER", TouchWidth*3/4-20, actionY+5, color.White)
}

func wrapScriptText(text string, width int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		words := strings.Fields(paragraph)
		line := ""
		for _, word := range words {
			if line != "" && len(line)+1+len(word) > width {
				lines = append(lines, line)
				line = word
			} else if line == "" {
				line = word
			} else {
				line += " " + word
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func (g *Game) scriptCoordinates(x, y int) (int, int) {
	if g.worldPhase&2 != 0 {
		x = g.worldWidth - 1 - x
	}
	if g.worldPhase&1 != 0 {
		y = g.worldHeight - 1 - y
	}
	return x, y
}

func (g *Game) scriptTeleport(playerIndex, x, y int) {
	x, y = g.scriptCoordinates(x, y)
	g.setPlayerTile(playerIndex, x, y)
}

func (g *Game) setPlayerTile(playerIndex, x, y int) {
	if playerIndex >= 0 && playerIndex < len(g.players) && x >= 0 && y >= 0 && x < g.worldWidth && y < g.worldHeight {
		g.players[playerIndex].x, g.players[playerIndex].y = x*16, y*16
		g.followPlayer()
	}
}

func (g *Game) spawnScriptEntity(kind, x, y, direction int, materialize bool) {
	x, y = g.scriptCoordinates(x, y)
	wait := 0
	if materialize {
		wait = 21
	}
	g.spawnEntityState(kind, x*16, y*16, direction, 0, -1, wait, wait)
}

func (g *Game) scriptDirection(direction int) int {
	if g.worldPhase&2 != 0 {
		switch direction {
		case 1:
			direction = 3
		case 3:
			direction = 1
		}
	}
	if g.worldPhase&1 != 0 {
		switch direction {
		case 0:
			direction = 2
		case 2:
			direction = 0
		}
	}
	return direction
}

func (s *scriptRuntime) word(index int) int16 {
	offset := numericVariableOffset + index*2
	if offset < 0 || offset+2 > len(s.state) {
		return 0
	}
	return int16(binary.BigEndian.Uint16(s.state[offset:]))
}

func (s *scriptRuntime) setWord(index int, value int16) {
	offset := numericVariableOffset + index*2
	if offset >= 0 && offset+2 <= len(s.state) {
		binary.BigEndian.PutUint16(s.state[offset:], uint16(value))
	}
}

func (s *scriptRuntime) stringValue(index int) string {
	offset := index * 20
	if offset < 0 || offset+20 > len(s.state) {
		return ""
	}
	data := s.state[offset : offset+20]
	if end := strings.IndexByte(string(data), 0); end >= 0 {
		data = data[:end]
	}
	return string(data)
}

func (s *scriptRuntime) setString(index int, value string) {
	offset := index * 20
	if offset < 0 || offset+20 > len(s.state) {
		return
	}
	clear(s.state[offset : offset+20])
	copy(s.state[offset:offset+19], value)
}

func (s *scriptRuntime) setStringByte(index, position int, value byte) {
	offset := index*20 + position
	if offset >= 0 && offset < len(s.state) {
		s.state[offset] = value
	}
}

func (s *scriptRuntime) randomizeString(destination, source int, g *Game) {
	destinationOffset, sourceOffset := destination*20, source*20
	if destinationOffset < 0 || sourceOffset < 0 || destinationOffset+20 > len(s.state) || sourceOffset+20 > len(s.state) {
		return
	}
	for index := range 20 {
		limit := s.state[sourceOffset+index]
		if limit == 0 {
			break
		}
		s.state[destinationOffset+index] += byte((g.nextRandom() & 0x7fff) % uint32(limit))
	}
}
