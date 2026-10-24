// Package game implements the native Time Bandits rules and presentation.
package game

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"math/bits"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"github.com/olivierh59500/timebandits/internal/original"
	"github.com/olivierh59500/timebandits/internal/sound"
)

const (
	TicksPerSecond     = 20
	ScreenWidth        = 320
	ScreenHeight       = 200
	TouchWidth         = 480
	TouchGameX         = 96
	TouchRightX        = TouchGameX + ScreenWidth
	TouchStickX        = TouchGameX / 2
	TouchControlY      = 125
	TouchFireX         = TouchRightX + (TouchWidth-TouchRightX)/2
	TouchPauseX        = TouchRightX + 4
	TouchPauseY        = 20
	TouchPauseWidth    = TouchWidth - TouchRightX - 8
	TouchPauseHeight   = 28
	ViewSize           = 176
	TwoViewWidth       = 144
	TwoViewHeight      = 112
	palettePhases      = 12
	poweredPhaseBase   = palettePhases
	worldPalettes      = palettePhases * 2
	gameOverDelayTicks = 20
	// The original holds the message for 114 pairs of PAL vertical blanks.
	gameOverMessageTicks  = 2 * (0x73 - 1) * TicksPerSecond / 50
	gameOverDurationTicks = gameOverDelayTicks + gameOverMessageTicks
)

const (
	pausePrompt    = "PRESS SPACEBAR..."
	gameOverPrompt = "YOU HAVE DIED...THIS ADVENTURE IS OVER."
)

type scene uint8

const (
	titleScene scene = iota
	playScene
	heroesScene
	heroDetailScene
	heroNameScene
)

// Options selects platform presentation and host-provided storage.
type Options struct {
	Touch    bool
	Demo     bool
	Mute     bool
	Players  int
	Smoke    int
	Capture  string
	SavePath string
}

type player struct {
	x, y          int
	direction     int
	frame         int
	power         int
	rating        int
	ratingScore   int
	killBonus     int
	ratingWait    int
	ratingTotal   uint64
	ratingSamples uint32
	trailCursor   int
	trailPosition [32]uint16
	trailVisits   [32]uint16
	score         int
	scorePending  int
	key           bool
	fireWait      int
	hurtWait      int
	scriptWait    int
	shadow        bool
	active        bool
}

type bullet struct {
	x, y, dx, dy int
	owner        int
	direction    int
	frame        int
	life         int
	active       bool
}

type entity struct {
	x, y                      int
	kind, direction, frame    int
	speed, sequence, vitality int
	value, firstFrame         int
	previousDirection         int
	successorKind             int
	splitCount                int
	flags                     uint16
	movement                  uint32
	graphics                  uint32
	age                       int
	life                      int
	spawnWait                 int
	materialize               int
	contactWait               int
	fireWait                  int
	active                    bool
}

var banditAnimationFrames = [2][32]int{
	{
		1, 1, 2, 2, 1, 1, 0, 0,
		9, 10, 11, 6, 7, 8, -1, -1,
		4, 4, 5, 5, 4, 4, 3, 3,
		15, 16, 17, 12, 13, 14, -1, -1,
	},
	{
		1, 1, 2, 2, 1, 1, 0, 0,
		6, 7, 8, 9, 8, 7, -1, -1,
		4, 4, 5, 5, 4, 4, 3, 3,
		10, 11, 12, 13, 12, 11, -1, -1,
	},
}

// Game is the shared desktop and Android implementation.
type Game struct {
	resources       *original.Resources
	sounds          *sound.Bank
	options         Options
	scene           scene
	tick            int
	idle            int
	paused          bool
	demo            bool
	demoVisited     []bool
	demoWorldTick   int
	demoTargetWorld int
	demoLastX       int
	demoLastY       int
	demoStall       int
	demoGoalX       int
	demoGoalY       int
	demoHasGoal     bool
	demoDialogPC    int
	demoDialogAt    int
	timeLords       [heroCount]heroRecord
	todaysHeroes    [heroCount]heroRecord
	heroQueue       []pendingHero
	heroInput       []rune
	heroDetail      int
	journal         bool
	gameOver        int
	cameraX         [2]int
	cameraY         [2]int
	worldIndex      int
	worldPhase      int
	worldWidth      int
	worldHeight     int
	worldPhases     [original.WorldCount]uint8
	worldJournal    [original.WorldCount]uint8
	worldDone       [original.WorldCount]bool
	worldVisited    [original.WorldCount]bool
	worldNames      []string
	controllers     [2]int
	players         [2]player
	mapData         []byte
	bullets         []bullet
	entities        []entity
	treasure        int
	pellets         int
	powerTimer      int
	powerCombo      int
	random          uint32
	spawnCursor     int
	deathToggle     int
	savePath        string
	saveSlots       [2]*savedGame
	savesLoaded     bool
	script          scriptRuntime
	dialog          *scriptDialog
	title           [12]*ebiten.Image
	hud             *ebiten.Image
	hudLogo         [worldPalettes]*ebiten.Image
	lordsLogo       [12]*ebiten.Image
	heroesLogo      [12]*ebiten.Image
	heroIcons       [12]*ebiten.Image
	heroArtifacts   [12]*ebiten.Image
	world           *ebiten.Image
	touchGame       *ebiten.Image
	spawnMasks      *ebiten.Image
	banditMasks     *ebiten.Image
	maskedActor     *ebiten.Image
	fonts           [3][2]*ebiten.Image
	tiles           [worldPalettes]*ebiten.Image
	composites      [worldPalettes]*ebiten.Image
	actors          [worldPalettes]*ebiten.Image
	bandits         [worldPalettes]*ebiten.Image
	icons           [worldPalettes]*ebiten.Image
	artifacts       [worldPalettes]*ebiten.Image
	paletteData     []byte
	paletteDirty    bool
	touchDirection  int
	touchFire       bool
	capture         bool
	done            bool
	err             error
}

// New loads checked, embedded resources and starts at the original game title.
func New(options Options) (*Game, error) {
	resources, err := original.Load()
	if err != nil {
		return nil, err
	}
	spawnMasks, err := original.DecodeMasks(resources.SpawnMasks)
	if err != nil {
		return nil, err
	}
	banditMasks, err := original.DecodeInverseMasks(resources.BanditOne)
	if err != nil {
		return nil, err
	}
	g := &Game{
		resources:      resources,
		options:        options,
		savePath:       options.SavePath,
		touchDirection: -1,
		controllers:    [2]int{2, 1},
		hud:            ebiten.NewImageFromImage(resources.HUD),
		world:          ebiten.NewImage(ViewSize, ViewSize),
		touchGame:      ebiten.NewImage(ScreenWidth, ScreenHeight),
		spawnMasks:     ebiten.NewImageFromImage(spawnMasks),
		banditMasks:    ebiten.NewImageFromImage(banditMasks),
		maskedActor:    ebiten.NewImage(16, 16),
	}
	for phase := range palettePhases {
		titleLogo, err := original.DecodePlanarGraphic(resources.TitleLogo, original.TitleLogoWidth, original.TitleLogoHeight, resources.Palette(0, phase))
		if err != nil {
			return nil, err
		}
		title := ebiten.NewImageFromImage(resources.Title)
		titleOptions := &ebiten.DrawImageOptions{}
		titleOptions.GeoM.Translate(16, 11)
		title.DrawImage(ebiten.NewImageFromImage(titleLogo), titleOptions)
		g.title[phase] = title
		timeLords, err := original.DecodePlanarGraphic(resources.TimeLordsLogo, original.TimeLordsLogoWidth, original.TimeLordsLogoHeight, resources.Palette(1, phase))
		if err != nil {
			return nil, err
		}
		todaysHeroes, err := original.DecodePlanarGraphic(resources.TodaysHeroesLogo, original.TodaysHeroesLogoWidth, original.TodaysHeroesLogoHeight, resources.Palette(1, phase))
		if err != nil {
			return nil, err
		}
		heroIcons, err := original.DecodeTiles(resources.Icons, resources.Palette(1, phase))
		if err != nil {
			return nil, err
		}
		heroArtifacts, err := original.DecodeTiles(resources.Artifacts, resources.Palette(1, phase))
		if err != nil {
			return nil, err
		}
		g.lordsLogo[phase] = ebiten.NewImageFromImage(timeLords)
		g.heroesLogo[phase] = ebiten.NewImageFromImage(todaysHeroes)
		g.heroIcons[phase] = ebiten.NewImageFromImage(heroIcons)
		g.heroArtifacts[phase] = ebiten.NewImageFromImage(heroArtifacts)
	}
	if g.savePath == "" {
		g.savePath = defaultSavePath()
	}
	g.todaysHeroes = defaultTodaysHeroes()
	if err := g.ensureSavesLoaded(); err != nil {
		return nil, err
	}
	if !options.Mute {
		g.sounds = sound.New(resources.Effects)
	}
	g.worldNames, err = resources.WorldNames()
	if err != nil {
		return nil, err
	}
	if err := g.loadWorld(0); err != nil {
		return nil, err
	}
	g.fonts = buildFonts(resources.Fonts)
	if options.Players > 0 {
		g.start(options.Players, options.Demo)
	} else if options.Demo {
		g.start(1, true)
	}
	return g, nil
}

func (g *Game) loadWorld(index int) error {
	if index < 0 || index >= original.WorldCount {
		return fmt.Errorf("world index %d is outside recovered data", index)
	}
	worldPhase := int(g.worldPhases[index])
	g.paletteData = append(g.paletteData[:0], g.resources.WorldRuntimePalette(index, worldPhase)...)
	g.paletteDirty = false
	if err := g.buildWorldAtlases(index, worldPhase); err != nil {
		return err
	}
	world, err := g.resources.World(index, worldPhase)
	if err != nil {
		return err
	}
	g.worldIndex, g.worldPhase = index, worldPhase
	g.worldWidth, g.worldHeight = world.Width, world.Height
	g.random = world.Random
	g.mapData = append(g.mapData[:0], g.resources.WorldMap(index, worldPhase)...)
	for index := range g.cameraX {
		g.cameraX[index], g.cameraY[index] = world.CameraX*4, world.CameraY*4
	}
	for playerIndex := range g.players {
		if g.players[playerIndex].active || playerIndex == 0 {
			if playerIndex == 0 {
				g.players[playerIndex].x = world.PlayerX * 4
				g.players[playerIndex].y = world.PlayerY * 4
				continue
			}
			x, y := 19, 5
			if worldPhase&2 != 0 {
				x = world.Width - 1 - x
			}
			if worldPhase&1 != 0 {
				y = world.Height - 1 - y
			}
			g.players[playerIndex].x, g.players[playerIndex].y = x*16, y*16
		}
		g.players[playerIndex].resetTrail()
	}
	g.bullets = g.bullets[:0]
	if cap(g.entities) < original.EntityCount {
		g.entities = make([]entity, original.EntityCount)
	} else {
		g.entities = g.entities[:original.EntityCount]
		clear(g.entities)
	}
	g.treasure, g.pellets = 0, 0
	g.powerTimer, g.powerCombo = 0, 0
	g.spawnCursor = 0
	g.deathToggle = 0
	g.loadScript(index, worldPhase)
	data := g.resources.WorldEntities(index, worldPhase)
	for slot := 0; slot < original.EntityCount; slot++ {
		record := data[slot*original.EntitySize : (slot+1)*original.EntitySize]
		if int16(binary.BigEndian.Uint16(record)) < 0 {
			continue
		}
		kind := int(int16(binary.BigEndian.Uint16(record[16:])))
		property, err := g.resources.EntityProperty(kind)
		if err != nil {
			return err
		}
		vitality := 0
		if property.Flags&0x100 != 0 {
			vitality = int(property.Vitality)
		}
		g.entities[slot] = entity{
			x:                 int(int16(binary.BigEndian.Uint16(record[8:]))) * 4,
			y:                 int(int16(binary.BigEndian.Uint16(record[10:]))) * 4,
			kind:              kind,
			direction:         int(binary.BigEndian.Uint16(record[18:])),
			frame:             int(binary.BigEndian.Uint16(record[14:])),
			speed:             int(binary.BigEndian.Uint16(record[12:])),
			sequence:          int(property.Sequence),
			vitality:          vitality,
			value:             int(binary.BigEndian.Uint16(record[48:])),
			firstFrame:        -1,
			previousDirection: int(binary.BigEndian.Uint16(record[68:])),
			successorKind:     int(binary.BigEndian.Uint16(record[62:])),
			splitCount:        int(binary.BigEndian.Uint16(record[60:])),
			flags:             binary.BigEndian.Uint16(record[46:]),
			movement:          binary.BigEndian.Uint32(record[20:]),
			graphics:          binary.BigEndian.Uint32(record[0x2a:]),
			life:              int(int16(binary.BigEndian.Uint16(record[32:]))),
			spawnWait:         int(binary.BigEndian.Uint16(record[52:])),
			materialize:       max(0, -int(int16(binary.BigEndian.Uint16(record[58:])))),
			contactWait:       int(binary.BigEndian.Uint16(record[64:])),
			active:            true,
		}
	}
	if index == 0 {
		// The Timegates startup script derives closed portals and the ending marker
		// from the phase counters that persist between world transitions.
		g.script.setWord(3, 0)
		g.runScript(g.script.base, 0, 0)
	}
	if g.demo {
		g.demoVisited = make([]bool, g.worldWidth*g.worldHeight)
		g.demoWorldTick, g.demoStall = 0, 0
		g.demoHasGoal = false
		g.demoDialogPC, g.demoDialogAt = 0, 0
		g.demoLastX, g.demoLastY = g.players[0].x, g.players[0].y
		if index > 0 {
			g.demoTargetWorld = index%16 + 1
		} else if g.demoTargetWorld == 0 {
			g.demoTargetWorld = 1
		}
	}
	return nil
}

func (g *Game) buildWorldAtlases(index, worldPhase int) error {
	compositeData := g.resources.WorldCompositeTiles(index, worldPhase)
	for animationPhase := range palettePhases {
		normal := g.resources.Colors(g.paletteData, animationPhase)
		powered := append(color.Palette(nil), normal...)
		powered[15] = color.NRGBA{0, 0, 238, 255}
		for variant, palette := range [...]color.Palette{normal, powered} {
			atlasPhase := animationPhase + variant*poweredPhaseBase
			tiles, err := original.DecodeTiles(g.resources.Tiles, palette)
			if err != nil {
				return err
			}
			g.tiles[atlasPhase] = ebiten.NewImageFromImage(tiles)
			composites, err := original.DecodeTiles(compositeData, palette)
			if err != nil {
				return err
			}
			g.composites[atlasPhase] = ebiten.NewImageFromImage(composites)
			transparent := append(color.Palette(nil), palette...)
			transparent[0] = color.NRGBA{}
			actors, err := original.DecodeTiles(g.resources.Actors, transparent)
			if err != nil {
				return err
			}
			g.actors[atlasPhase] = ebiten.NewImageFromImage(actors)
			bandits, err := original.DecodeTiles(g.resources.BanditTwo, palette)
			if err != nil {
				return err
			}
			g.bandits[atlasPhase] = ebiten.NewImageFromImage(bandits)
			icons, err := original.DecodeTiles(g.resources.Icons, transparent)
			if err != nil {
				return err
			}
			g.icons[atlasPhase] = ebiten.NewImageFromImage(icons)
			artifacts, err := original.DecodeTiles(g.resources.Artifacts, transparent)
			if err != nil {
				return err
			}
			g.artifacts[atlasPhase] = ebiten.NewImageFromImage(artifacts)
			hudLogo, err := original.DecodePlanarGraphic(g.resources.HUDLogo, original.HUDLogoWidth, original.HUDLogoHeight, palette)
			if err != nil {
				return err
			}
			g.hudLogo[atlasPhase] = ebiten.NewImageFromImage(hudLogo)
		}
	}
	return nil
}

func buildFonts(raw []byte) [3][2]*ebiten.Image {
	const glyphs = 59
	var fonts [3][2]*ebiten.Image
	for style := range fonts {
		for layer := range fonts[style] {
			atlas := ebiten.NewImage(16*8, 4*10)
			for glyph := range glyphs {
				for y := range 10 {
					bits := raw[style*glyphs*20+glyph*20+y*2+layer]
					for x := range 8 {
						if bits&(1<<uint(7-x)) != 0 {
							atlas.Set(glyph%16*8+x, glyph/16*10+y, color.White)
						}
					}
				}
			}
			fonts[style][layer] = atlas
		}
	}
	return fonts
}

func initialPlayer(x, y, direction int, active bool) player {
	p := player{
		x: x, y: y, direction: direction, power: 14,
		rating: 5, ratingScore: 450, killBonus: 2, ratingWait: 99,
		ratingSamples: 1, active: active,
	}
	p.resetTrail()
	return p
}

func (p *player) resetTrail() {
	p.trailCursor = len(p.trailPosition) - 1
	clear(p.trailPosition[:])
	for index := range p.trailVisits {
		p.trailVisits[index] = 16
	}
	p.ratingWait = 99
}

func (g *Game) start(count int, demo bool) {
	g.scene = playScene
	g.demo = demo
	g.demoTargetWorld = 1
	g.paused = false
	g.journal = false
	g.tick = 0
	g.gameOver = 0
	g.players[0] = initialPlayer(5*16, 5*16, 2, true)
	g.players[1] = initialPlayer(6*16, 5*16, 3, count == 2)
	g.random = 0x12b9b09f
	g.bullets = g.bullets[:0]
	g.worldPhases = [original.WorldCount]uint8{}
	g.worldJournal = [original.WorldCount]uint8{}
	g.worldDone = [original.WorldCount]bool{}
	g.worldVisited = [original.WorldCount]bool{}
	g.script = scriptRuntime{}
	_ = g.loadWorld(0)
}

// Update advances one original PAL tick.
func (g *Game) Update() error {
	g.sounds.Update()
	if g.done {
		if g.err != nil {
			return g.err
		}
		return ebiten.Termination
	}
	if g.dialog != nil {
		if g.demo {
			g.updateDemoDialog()
		} else {
			g.updateDialog()
		}
		return nil
	}
	switch g.scene {
	case heroesScene:
		g.updateHeroes()
		return nil
	case heroDetailScene:
		g.updateHeroDetail()
		return nil
	case heroNameScene:
		g.updateHeroName()
		return nil
	}
	if g.gameOver == 0 && inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		if g.scene == playScene {
			if g.paused {
				g.scene = titleScene
				g.paused = false
			} else {
				g.paused = true
			}
		}
	}
	if g.scene == titleScene {
		g.updateTitle()
		return nil
	}
	if g.gameOver > 0 {
		g.gameOver--
		if g.gameOver == 0 {
			g.finishAdventure()
		}
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyP) {
		g.paused = !g.paused
	}
	if g.paused {
		if inpututil.IsKeyJustPressed(ebiten.KeySpace) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) || g.options.Touch && g.pauseTouchPressed() {
			g.paused = false
		}
		return nil
	}
	g.readTouch()
	g.tick++
	if g.powerTimer > 0 {
		g.powerTimer--
		if g.powerTimer == 0 {
			g.powerCombo = 0
		}
	}
	for index := range g.players {
		if g.players[index].scorePending > 0 {
			g.players[index].scorePending--
			g.players[index].score++
			if g.players[index].score%100 == 0 && g.players[index].power < 15 {
				g.players[index].power++
			}
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
	if g.demo {
		g.updateDemo()
	} else {
		g.updateHuman(0, ebiten.KeyArrowLeft, ebiten.KeyArrowRight, ebiten.KeyArrowUp, ebiten.KeyArrowDown, ebiten.KeySpace)
		if g.players[1].active {
			g.updateHuman(1, ebiten.KeyJ, ebiten.KeyL, ebiten.KeyI, ebiten.KeyK, ebiten.KeyO)
		}
	}
	g.recordPlayerTrails()
	g.updateBullets()
	if g.updateWorldCells() {
		g.followPlayer()
		return nil
	}
	g.updateScript()
	g.updateActors()
	g.updateEntityContacts()
	g.updateSpawns()
	g.updateEntities()
	g.updateRatings()
	g.followPlayer()
	if g.options.Smoke > 0 && g.tick >= g.options.Smoke {
		g.capture = true
	}
	return nil
}

func (g *Game) updateEntities() {
	if g.worldIndex != 0 {
		return
	}
	for playerIndex := range g.players {
		p := &g.players[playerIndex]
		if !p.active {
			continue
		}
		for index := range g.entities {
			object := &g.entities[index]
			if !object.active || object.kind < 24 || object.kind > 39 || abs(p.x-object.x) > 8 || abs(p.y-object.y) > 8 {
				continue
			}
			target := object.kind - 23
			if target < original.WorldCount && !g.worldDone[target] && g.activateExit(playerIndex, target) {
				return
			}
		}
	}
}

func (g *Game) updateActors() {
	clock := uint32(1) << uint(g.tick&31)
	type actorShot struct{ x, y, direction int }
	var shots []actorShot
	for index := range g.entities {
		object := &g.entities[index]
		if !object.active {
			continue
		}
		if g.entityVisible(object) {
			object.age = 0
		} else {
			object.age++
			if object.age > 224 && object.flags&0x20 == 0 {
				object.active = false
				continue
			}
		}
		if object.spawnWait > 0 {
			object.spawnWait--
		}
		if object.materialize > 0 {
			object.materialize--
		}
		if object.contactWait > 0 {
			object.contactWait--
		}
		if g.worldIndex == 0 || object.movement&clock == 0 {
			continue
		}
		if object.life > 0 {
			object.life--
			if object.life == 0 {
				object.active = false
				continue
			}
		}
		object.frame = (object.frame + 1) & 7
		if _, ended := g.resources.EntityAnimation(object.sequence, object.direction, object.frame); ended {
			object.frame = 0
		}
		if object.speed == 0 {
			continue
		}
		if object.spawnWait == 0 && object.x%16 == 0 && object.y%16 == 0 {
			object.previousDirection = object.direction
			if object.flags&0x0800 != 0 && index > 0 && g.entities[index-1].active {
				object.direction = g.entities[index-1].previousDirection
			} else if object.kind >= 42 && object.flags&0x0400 == 0 {
				direction, targeted := g.entityDirection(object)
				object.direction = direction
				if targeted && object.flags&8 != 0 && object.age <= 3 {
					shots = append(shots, actorShot{object.x, object.y, object.direction})
				}
			}
		}
		if g.moveEntity(object) {
			continue
		}
		if object.kind < 42 {
			if object.flags&0x40 != 0 {
				object.direction ^= 2
			} else {
				object.active = false
			}
			continue
		}
		if object.flags&0x0c00 != 0 {
			object.direction = g.segmentTurn(object)
			continue
		}
		object.direction = int(g.random & 3)
	}
	for _, shot := range shots {
		g.spawnTransientEntityState(5, shot.x, shot.y, shot.direction, 0, 10, 0, 0)
		g.playSound(3)
	}
}

func (g *Game) segmentTurn(object *entity) int {
	turns := [...]int{1, 3, 2}
	if g.random&0x20 != 0 {
		turns[0], turns[1] = turns[1], turns[0]
	}
	for _, turn := range turns {
		direction := (object.previousDirection + turn) & 3
		if g.entityStepAllowed(object, direction) {
			return direction
		}
	}
	return object.previousDirection ^ 2
}

func (g *Game) entityVisible(object *entity) bool {
	viewWidth, viewHeight := ViewSize, ViewSize
	if g.players[1].active {
		viewWidth, viewHeight = TwoViewWidth, TwoViewHeight
	}
	for index := range g.players {
		if !g.players[index].active {
			continue
		}
		x, y := object.x-g.cameraX[index], object.y-g.cameraY[index]
		if x > -16 && y > -16 && x < viewWidth && y < viewHeight {
			return true
		}
	}
	return false
}

func (g *Game) updateSpawns() {
	world, err := g.resources.World(g.worldIndex, g.worldPhase)
	if err != nil || world.SpawnCount == 0 {
		return
	}
	interval := 8
	if g.worldIndex == 5 {
		interval = 64
	} else if g.worldPhase == 0 {
		interval = 16
	} else if g.worldPhase > 9 {
		interval = 4
	}
	if g.tick%interval != 0 {
		return
	}
	poolSize := min(interval, original.EntityCount)
	slot := poolSize
	for index := 0; index < poolSize; index++ {
		if !g.entities[index].active {
			slot = index
			break
		}
	}
	if slot >= original.EntityCount {
		return
	}
	spawn, err := g.resources.WorldSpawn(g.worldIndex, g.worldPhase, g.spawnCursor%world.SpawnCount)
	if err != nil {
		return
	}
	g.spawnCursor = (g.spawnCursor + 1) % world.SpawnCount
	kinds, err := g.resources.WorldSpawnKinds(g.worldIndex, g.worldPhase)
	if err != nil {
		return
	}
	random := g.nextRandom()
	kind := int(kinds[(random>>16)&7])
	spawnWait := 20
	if g.worldPhase <= 3 {
		spawnWait = 35
	}
	spawned, ok := g.makeEntityState(kind, spawn.X*16, spawn.Y*16, int(random&3), int(random&7), -1, spawnWait, 21)
	if ok {
		g.entities[slot] = spawned
	}
}

func (g *Game) entityDirection(object *entity) (int, bool) {
	if object.flags&1 != 0 {
		if direction, ok := g.directionFromPlayerTrail(object); ok {
			return direction, false
		}
	}
	random := g.nextRandom()
	if random&(1<<5) != 0 {
		return object.direction, false
	}
	if g.tick&16 != 0 {
		if direction, ok := g.directionToOriginalTarget(object); ok {
			return direction, true
		}
	}
	return int(random & 3), false
}

func (g *Game) directionFromPlayerTrail(object *entity) (int, bool) {
	position := uint16(object.x/16)<<8 | uint16(object.y/16)
	for playerIndex := range g.players {
		p := &g.players[playerIndex]
		if !p.active {
			continue
		}
		cursor := p.trailCursor
		for range len(p.trailPosition) {
			if p.trailPosition[cursor] == position {
				next := p.trailPosition[(cursor+1)%len(p.trailPosition)]
				dx := int(next>>8) - object.x/16
				dy := int(next&0xff) - object.y/16
				switch {
				case dx == 0 && dy == -1:
					return 0, true
				case dx == 1 && dy == 0:
					return 1, true
				case dx == 0 && dy == 1:
					return 2, true
				case dx == -1 && dy == 0:
					return 3, true
				}
				break
			}
			cursor = (cursor + len(p.trailPosition) - 1) % len(p.trailPosition)
		}
	}
	return 0, false
}

func (g *Game) directionToOriginalTarget(object *entity) (int, bool) {
	var target *player
	if g.players[1].active && g.tick&32 == 0 {
		target = &g.players[1]
	} else if g.players[0].active {
		target = &g.players[0]
	} else if g.players[1].active {
		target = &g.players[1]
	}
	if target == nil {
		return 0, false
	}
	dx, dy := target.x-object.x, target.y-object.y
	if abs(dx) > abs(dy) {
		if dx < 0 {
			return 3, true
		}
		return 1, true
	}
	if dy < 0 {
		return 0, true
	}
	return 2, true
}

func (g *Game) moveEntity(object *entity) bool {
	if !g.entityStepAllowed(object, object.direction) {
		return false
	}
	vector := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}[object.direction]
	object.x += vector[0] * object.speed * 4
	object.y += vector[1] * object.speed * 4
	return true
}

func (g *Game) entityStepAllowed(object *entity, direction int) bool {
	vector := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}[direction]
	if vector[0] == 0 && object.x%16 != 0 || vector[1] == 0 && object.y%16 != 0 {
		return false
	}
	if vector[0] != 0 && object.x%16 != 0 || vector[1] != 0 && object.y%16 != 0 {
		return true
	}
	x, y := object.x/16+vector[0], object.y/16+vector[1]
	if x < 0 || y < 0 || x >= g.worldWidth || y >= g.worldHeight {
		return false
	}
	flags, tile := g.cell(x, y)
	collision, _ := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
	if collision < 0x92 || collision == 0xc9 || collision == 0xf4 {
		return false
	}
	if object.kind > 2 && collision >= 0xdd && collision <= 0xe6 {
		return false
	}
	return collision < 0xec || collision > 0xef || collision-0xec != byte(direction)
}

func (g *Game) updateEntityContacts() {
	for playerIndex := range g.players {
		p := &g.players[playerIndex]
		if !p.active || p.hurtWait != 0 {
			continue
		}
		for index := range g.entities {
			object := &g.entities[index]
			if !object.active || object.materialize > 0 || object.contactWait > 0 || object.kind >= 10 && object.kind <= 41 {
				continue
			}
			radius := 8
			if object.kind == 2 || object.kind == 6 || object.kind == 7 {
				radius = 4
			}
			if abs(p.x-object.x) > radius || abs(p.y-object.y) > radius {
				continue
			}
			if g.powerTimer > 0 {
				g.eatPoweredEntity(playerIndex, object)
				break
			}
			if object.flags&0x0200 != 0 {
				g.script.setWord(8, 1)
				break
			}
			x, y, direction := object.x, object.y, object.direction
			object.contactWait = 15
			object.vitality--
			if object.vitality < 0 {
				g.promoteNextSegment(index)
				object.active = false
			}
			g.damagePlayer(playerIndex)
			g.spawnTransientEntityState(12, x, y, direction, 0, 6, 0, 0)
			break
		}
	}
}

func (g *Game) eatPoweredEntity(playerIndex int, object *entity) {
	g.powerCombo = min(4, g.powerCombo+1)
	g.award(playerIndex, g.powerCombo*10)
	kind := 14 + g.powerCombo
	property, err := g.resources.EntityProperty(kind)
	if err != nil {
		object.active = false
		return
	}
	object.kind, object.frame = kind, 0
	object.speed, object.sequence = int(property.Speed), int(property.Sequence)
	object.value, object.firstFrame = int(property.Value), -1
	object.flags, object.movement = property.Flags, property.Movement
	object.graphics = 0x2ecd8 + uint32(property.Graphic)*original.TileSize
	object.vitality, object.life, object.age = 0, 30, 0
	object.successorKind, object.splitCount = int(property.Vitality&0xff), int(property.Vitality>>8)
	object.spawnWait, object.materialize, object.contactWait = 0, 0, 0
	g.playSound(7)
}

func (g *Game) updateWorldCells() bool {
	for playerIndex := range g.players {
		p := &g.players[playerIndex]
		if !p.active || p.x%16 != 0 || p.y%16 != 0 {
			continue
		}
		x, y := p.x/16, p.y/16
		flags, tile := g.cell(x, y)
		switch flags >> 5 & 3 {
		case 1:
			if p.scriptWait == 0 {
				p.scriptWait = 45
				g.runWorldTrigger(playerIndex, int(flags&0x1f), x, y)
				if g.dialog != nil {
					return false
				}
			}
		case 2:
			portal, err := g.resources.WorldPortal(g.worldIndex, g.worldPhase, int(flags&0x1f))
			if err == nil {
				p.x, p.y = portal.X*16, portal.Y*16
			}
		case 3:
			if g.activateExit(playerIndex, int(flags&0x1f)) {
				return true
			}
		default:
			switch tile {
			case 0xe9:
				g.award(playerIndex, 1)
				g.playSound(4)
				g.setCell(x, y, flags, 0x92)
				g.pellets++
				if g.pellets == 181 && g.activateExit(playerIndex, 0) {
					return true
				}
			case 0xea:
				g.award(playerIndex, 5)
				g.playSound(9)
				g.setCell(x, y, flags, 0x92)
				g.powerTimer = max(0, 130-g.worldPhase*6)
				g.pellets++
				if g.pellets == 181 && g.activateExit(playerIndex, 0) {
					return true
				}
			case 0xe7, 0xe8:
				if !p.shadow {
					g.setCell(x, y, flags, 0x9d)
					g.damagePlayer(playerIndex)
				}
			}
		}
	}
	return false
}

func (g *Game) activateExit(playerIndex, exitIndex int) bool {
	return g.activateExitMode(playerIndex, exitIndex, true)
}

func (g *Game) activateExitMode(playerIndex, exitIndex int, recordJournal bool) bool {
	exit, err := g.resources.WorldExit(g.worldIndex, g.worldPhase, exitIndex)
	if err != nil || exit.World < 0 || exit.World >= original.WorldCount {
		return false
	}
	oldWorld := g.worldIndex
	if oldWorld != 0 {
		storedPhase := int(g.worldPhases[oldWorld])
		if recordJournal {
			g.worldJournal[oldWorld] = uint8(storedPhase)
			g.worldVisited[oldWorld] = true
		}
		if storedPhase == original.PhaseCount-1 {
			g.worldPhases[oldWorld] = 0xff
			g.worldDone[oldWorld] = true
		} else {
			g.worldPhases[oldWorld] = uint8(storedPhase + 1)
		}
	}
	if exit.World == 0 {
		g.worldPhases[0] = 0
	}
	if err := g.loadWorld(exit.World); err != nil {
		return false
	}
	g.placePlayers(exit)
	g.followPlayer()
	return true
}

func (g *Game) placePlayers(exit original.Exit) {
	positions := [2][2]int{{exit.OneX, exit.OneY}, {exit.TwoX, exit.TwoY}}
	for index := range g.players {
		if !g.players[index].active {
			continue
		}
		x, y := positions[index][0], positions[index][1]
		if g.worldPhase&2 != 0 {
			x = g.worldWidth - 1 - x
		}
		if g.worldPhase&1 != 0 {
			y = g.worldHeight - 1 - y
		}
		g.players[index].x, g.players[index].y = x*16, y*16
	}
}

func (g *Game) updateTitle() {
	g.tick++
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.controllers[0] = (g.controllers[0] + 1) % 3
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF2) {
		g.controllers[1] = (g.controllers[1] + 1) % 3
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyH) {
		g.scene = heroesScene
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDigit1) || inpututil.IsKeyJustPressed(ebiten.KeyZ) || inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.start(1, false)
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDigit2) {
		g.start(2, false)
		return
	}
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		if x < TouchGameX && y < 68 {
			g.scene = heroesScene
			return
		}
		if x >= TouchRightX {
			g.start(2, false)
		} else {
			g.start(1, false)
		}
		return
	}
	g.idle++
	if g.idle >= 15*TicksPerSecond {
		g.start(1, true)
	}
}

func (g *Game) updateDemo() {
	g.updateDemoPlayer()
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) || inpututil.IsKeyJustPressed(ebiten.KeyEscape) || len(ebiten.AppendTouchIDs(nil)) > 0 {
		g.scene = titleScene
		g.idle = 0
		g.demo = false
	}
}

func (g *Game) loadDemoWorld(world int) {
	if world == 0 {
		_ = g.loadWorld(0)
		return
	}
	exit, err := g.resources.WorldExit(0, 0, world)
	if err != nil || exit.World != world || g.loadWorld(world) != nil {
		return
	}
	g.placePlayers(exit)
	g.followPlayer()
}

func (g *Game) updateHuman(index int, left, right, up, down, fire ebiten.Key) {
	dx, dy := 0, 0
	if ebiten.IsKeyPressed(left) {
		dx--
	}
	if ebiten.IsKeyPressed(right) {
		dx++
	}
	if ebiten.IsKeyPressed(up) {
		dy--
	}
	if ebiten.IsKeyPressed(down) {
		dy++
	}
	firing := ebiten.IsKeyPressed(fire)
	if index == 0 {
		if ebiten.IsKeyPressed(ebiten.KeyA) {
			dx--
		}
		if ebiten.IsKeyPressed(ebiten.KeyD) {
			dx++
		}
		if ebiten.IsKeyPressed(ebiten.KeyW) {
			dy--
		}
		if ebiten.IsKeyPressed(ebiten.KeyS) {
			dy++
		}
		firing = firing || ebiten.IsKeyPressed(ebiten.KeyZ)
	}
	gamepads := ebiten.AppendGamepadIDs(nil)
	gamepadIndex := controllerGamepadIndex(g.controllers[index])
	if gamepadIndex >= 0 && gamepadIndex < len(gamepads) && ebiten.IsStandardGamepadLayoutAvailable(gamepads[gamepadIndex]) {
		id := gamepads[gamepadIndex]
		const threshold = 0.35
		horizontal := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal)
		vertical := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical)
		if horizontal < -threshold || ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftLeft) {
			dx--
		}
		if horizontal > threshold || ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftRight) {
			dx++
		}
		if vertical < -threshold || ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftTop) {
			dy--
		}
		if vertical > threshold || ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonLeftBottom) {
			dy++
		}
		firing = firing || ebiten.IsStandardGamepadButtonPressed(id, ebiten.StandardGamepadButtonRightBottom)
	}
	dx, dy = max(-1, min(1, dx)), max(-1, min(1, dy))
	if index == 0 && g.options.Touch {
		dx, dy = touchDirectionVector(g.touchDirection)
		firing = firing || g.touchFire
	}
	if firing && (dx != 0 || dy != 0) {
		g.players[index].direction = direction(dx, dy)
		g.players[index].frame = 0
	} else if dx == 0 && dy == 0 {
		g.players[index].frame = 0
	} else if index == 0 && g.options.Touch {
		g.moveTouch(index, dx, dy)
	} else {
		g.move(index, dx, dy)
	}
	if firing {
		g.fire(index)
	}
}

func controllerGamepadIndex(controller int) int {
	switch controller {
	case 1:
		return 1
	case 2:
		return 0
	default:
		return -1
	}
}

func (g *Game) moveTouch(index, dx, dy int) {
	if dx == 0 && dy == 0 {
		return
	}
	p := &g.players[index]
	requested := direction(dx, dy)
	if requested&1 == 0 && p.x%16 != 0 {
		correction := 1
		if p.direction == 3 || p.direction&1 == 0 && p.x%16 <= 8 {
			correction = -1
		}
		g.move(index, correction, 0)
		return
	}
	if requested&1 != 0 && p.y%16 != 0 {
		correction := 1
		if p.direction == 0 || p.direction&1 != 0 && p.y%16 <= 8 {
			correction = -1
		}
		g.move(index, 0, correction)
		return
	}
	g.move(index, dx, dy)
}

func (g *Game) move(index, dx, dy int) {
	if dx == 0 && dy == 0 {
		g.players[index].frame = 0
		return
	}
	p := &g.players[index]
	direction := direction(dx, dy)
	p.direction = direction
	vector := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}[direction]
	if vector[0] == 0 && p.x%16 != 0 || vector[1] == 0 && p.y%16 != 0 {
		return
	}
	tileX, tileY := p.x/16, p.y/16
	atBoundary := vector[0] < 0 && p.x%16 == 0 || vector[0] > 0 && p.x%16 == 0 || vector[1] < 0 && p.y%16 == 0 || vector[1] > 0 && p.y%16 == 0
	if atBoundary && !g.enterCell(index, tileX+vector[0], tileY+vector[1], direction) {
		p.frame = 0
		return
	}
	p.x += vector[0] * 4
	p.y += vector[1] * 4
	if g.worldIndex == 14 {
		p.frame = 0
	} else {
		advanceBanditAnimation(p)
	}
}

func advanceBanditAnimation(p *player) {
	p.frame = (p.frame + 1) & 7
	shadow := 0
	if p.shadow {
		shadow = 1
	}
	if banditAnimationFrames[shadow][p.direction*8+p.frame] < 0 {
		p.frame = 0
	}
}

func banditAnimationFrame(p *player) int {
	shadow := 0
	if p.shadow {
		shadow = 1
	}
	frame := banditAnimationFrames[shadow][p.direction*8+p.frame]
	if frame < 0 {
		return banditAnimationFrames[shadow][p.direction*8]
	}
	return frame
}

func direction(dx, dy int) int {
	if dy < 0 {
		return 0
	}
	if dx > 0 {
		return 1
	}
	if dy > 0 {
		return 2
	}
	return 3
}

func (g *Game) walkable(x, y int) bool {
	if x < 0 || y < 0 || x+15 >= g.worldWidth*16 || y+15 >= g.worldHeight*16 {
		return false
	}
	for _, point := range [][2]int{{x + 3, y + 3}, {x + 12, y + 3}, {x + 3, y + 12}, {x + 12, y + 12}} {
		flags, tile := g.cell(point[0]/16, point[1]/16)
		collision, _ := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
		if collision < 0x92 || collision == 0xeb || collision == 0xf4 && !g.players[0].key {
			return false
		}
	}
	return true
}

func (g *Game) enterCell(playerIndex, x, y, direction int) bool {
	if x < 0 || y < 0 || x >= g.worldWidth || y >= g.worldHeight {
		return false
	}
	flags, tile := g.cell(x, y)
	collision, replacement := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
	if collision < 0x92 {
		return false
	}
	p := &g.players[playerIndex]
	switch collision {
	case 0xeb:
		variant := byte(g.nextRandom() & 3)
		for variant == 0 {
			variant = byte(g.nextRandom() & 3)
		}
		g.setCell(x, y, flags, 0x8e+variant)
		g.award(playerIndex, 1)
		return false
	case 0xf5:
		if p.key {
			return false
		}
		p.key = true
		g.playSound(5)
		g.replaceCell(x, y, flags, replacement)
		g.award(playerIndex, 15)
	case 0xf4:
		if !p.key {
			return false
		}
		p.key = false
		g.playSound(6)
		g.replaceCell(x, y, flags, replacement)
		g.award(playerIndex, 25)
	case 0xf0, 0xf1, 0xf2, 0xf3:
		g.replaceCell(x, y, flags, replacement)
		g.treasure += 10
		g.award(playerIndex, g.treasure)
		g.playSound(7)
	default:
		if collision >= 0xec && byte(direction) == collision-0xec {
			return false
		}
	}
	return true
}

func (g *Game) award(playerIndex, cubitUnits int) {
	if playerIndex < 0 || playerIndex >= len(g.players) || cubitUnits <= 0 || g.players[playerIndex].shadow {
		return
	}
	g.players[playerIndex].scorePending += cubitUnits
}

func (g *Game) cell(x, y int) (flags, tile byte) {
	if x < 0 || x >= 64 || y < 0 || y >= 64 || len(g.mapData) != original.MapCapacity {
		return 0xff, 0
	}
	offset := y*original.MapStride + x*2
	return g.mapData[offset], g.mapData[offset+1]
}

func (g *Game) setCell(x, y int, flags, tile byte) {
	offset := y*original.MapStride + x*2
	g.mapData[offset], g.mapData[offset+1] = flags, tile
}

func (g *Game) replaceCell(x, y int, flags, replacement byte) {
	g.setCell(x, y, flags&0x7f, replacement)
}

func (g *Game) nextRandom() uint32 {
	g.random *= 125
	if g.random == 0 {
		g.random = 0x12b9b09f
	}
	return g.random >> 3
}

func (g *Game) recordPlayerTrails() {
	for index := range g.players {
		p := &g.players[index]
		if !p.active || p.x%16 != 0 || p.y%16 != 0 {
			continue
		}
		position := uint16(p.x/16)<<8 | uint16(p.y/16)
		if p.trailPosition[p.trailCursor] != position {
			p.trailCursor = (p.trailCursor + 1) % len(p.trailPosition)
			p.trailPosition[p.trailCursor] = position
			p.trailVisits[p.trailCursor] = 15
		}
		if p.trailVisits[p.trailCursor] < 30000 {
			p.trailVisits[p.trailCursor]++
		}
	}
}

func (g *Game) ratingMetric(p *player) int {
	reference := p.trailPosition[p.trailCursor]
	referenceX, referenceY := int(reference>>8), int(reference&0xff)
	metric := 0
	cursor := p.trailCursor
	for age := 31; age >= 0; age-- {
		position := p.trailPosition[cursor]
		distance := abs(referenceX-int(position>>8)) + abs(referenceY-int(position&0xff))
		contribution := int(p.trailVisits[cursor])
		if distance < 12 {
			contribution *= g.resources.RatingDistanceWeight(distance)
			contribution = min(contribution, 65000)
		}
		contribution /= g.resources.RatingAgeDivisor(age)
		metric += contribution
		cursor = (cursor + len(p.trailPosition) - 1) % len(p.trailPosition)
	}
	return min(metric, 30000)
}

func (g *Game) updateRatings() {
	if g.worldIndex == 0 || g.worldIndex == 5 {
		return
	}
	for index := range g.players {
		p := &g.players[index]
		if !p.active || p.shadow {
			continue
		}
		p.ratingWait++
		current, err := g.resources.RatingProfile(p.rating)
		if err != nil {
			continue
		}
		metric := g.ratingMetric(p)
		p.ratingScore += g.resources.RatingPerformanceCategory(metric) + current.Drift
		p.ratingScore = max(0, min(1000, p.ratingScore))
		newRating := g.resources.RatingForScore(p.ratingScore)
		updated, err := g.resources.RatingProfile(newRating)
		if err != nil {
			continue
		}
		p.killBonus = updated.KillBonus
		p.ratingTotal += uint64(p.ratingScore)
		p.ratingSamples++
		if g.tick >= 2 && newRating != p.rating && p.ratingWait >= 15 {
			p.rating = newRating
			p.ratingWait = 0
		}
	}
}

func (g *Game) ratingText(p *player) string {
	profile, err := g.resources.RatingProfile(p.rating)
	if err != nil {
		return "AVERAGE  (20)"
	}
	return fmt.Sprintf("%-9s (%d)", profile.Name, profile.KillBonus*10)
}

func (g *Game) spawnEntityState(kind, x, y, direction, frame, life, spawnWait, materialize int) {
	spawned, ok := g.makeEntityState(kind, x, y, direction, frame, life, spawnWait, materialize)
	if !ok {
		return
	}
	for index := range g.entities {
		if !g.entities[index].active {
			g.entities[index] = spawned
			return
		}
	}
	if len(g.entities) < original.EntityCount {
		g.entities = append(g.entities, spawned)
	}
}

func (g *Game) spawnTransientEntityState(kind, x, y, direction, frame, life, spawnWait, materialize int) {
	const poolSize = 56
	spawned, ok := g.makeEntityState(kind, x, y, direction, frame, life, spawnWait, materialize)
	if !ok {
		return
	}
	slot := poolSize
	for index := 0; index < poolSize; index++ {
		if index >= len(g.entities) || !g.entities[index].active {
			slot = index
			break
		}
	}
	if slot >= original.EntityCount {
		return
	}
	if slot >= len(g.entities) {
		g.entities = append(g.entities, make([]entity, slot-len(g.entities)+1)...)
	}
	g.entities[slot] = spawned
}

func (g *Game) makeEntityState(kind, x, y, direction, frame, life, spawnWait, materialize int) (entity, bool) {
	property, err := g.resources.EntityProperty(kind)
	if err != nil {
		return entity{}, false
	}
	vitality := 0
	if property.Flags&0x100 != 0 {
		vitality = int(property.Vitality)
	}
	spawned := entity{
		x: x, y: y, kind: kind, direction: direction & 3, frame: frame & 7,
		speed: int(property.Speed), sequence: int(property.Sequence), vitality: vitality,
		value: int(property.Value), firstFrame: -1,
		previousDirection: direction & 3,
		successorKind:     int(property.Vitality & 0xff), splitCount: int(property.Vitality >> 8),
		flags: property.Flags, movement: g.runtimeMovement(kind, property),
		graphics: 0x2ecd8 + uint32(property.Graphic)*original.TileSize,
		life:     life, spawnWait: spawnWait, materialize: materialize, active: true,
	}
	return spawned, true
}

func (g *Game) runtimeMovement(kind int, property original.EntityProperty) uint32 {
	movement := property.Movement
	if g.worldIndex != 0 && g.worldPhase == 0 && kind >= 42 {
		if g.tick <= 1800 {
			movement &= 0x55555555
		}
		if g.tick <= 500 {
			movement &= 0x11111111
		}
	}
	rotation := int(g.nextRandom() & 31)
	if property.Flags&0x0c00 == 0 {
		movement = bits.RotateLeft32(movement, rotation)
	}
	return movement
}

func (g *Game) fire(index int) {
	p := &g.players[index]
	active := 0
	for shot := range g.bullets {
		if g.bullets[shot].owner == index {
			active++
		}
	}
	if !p.active || p.fireWait != 0 || active >= 2 {
		return
	}
	vector := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}[p.direction]
	g.bullets = append(g.bullets, bullet{
		x: p.x, y: p.y, dx: vector[0] * 8, dy: vector[1] * 8,
		owner: index, direction: p.direction, life: 12, active: true,
	})
	p.fireWait = 3
	g.playSound(index)
}

func (g *Game) updateBullets() {
	kept := g.bullets[:0]
	for _, shot := range g.bullets {
		if shot.life > 0 {
			shot.life--
			if shot.life == 0 {
				continue
			}
		}
		shot.x += shot.dx
		shot.y += shot.dy
		shot.frame = (shot.frame + 1) & 7
		if !g.projectilePassable(shot) {
			continue
		}
		hit := false
		ricochet := false
		fragments := false
		fragmentX, fragmentY := 0, 0
		splitKind, splitCount, splitDirection := 0, 0, 0
		for index := range g.entities {
			object := &g.entities[index]
			if !object.active || object.materialize > 0 || object.kind <= 41 || abs(shot.x-object.x) > 8 || abs(shot.y-object.y) > 8 {
				continue
			}
			hit = true
			g.playSound(2)
			impactFlags := object.flags
			if impactFlags&0x10 == 0 {
				object.vitality--
				if object.vitality < 0 {
					fragments = impactFlags&2 != 0
					fragmentX, fragmentY = object.x, object.y
					if impactFlags&0x80 != 0 {
						splitKind, splitCount, splitDirection = object.successorKind, object.splitCount, shot.direction
					}
					g.promoteNextSegment(index)
					g.transformEntityDeath(object, shot.direction)
					g.award(shot.owner, g.players[shot.owner].killBonus)
				}
			}
			ricochet = impactFlags&4 != 0
			break
		}
		if hit {
			g.spawnTransientEntityState(12, shot.x, shot.y, shot.direction, 0, 6, 0, 0)
			if fragments {
				g.spawnFragments(fragmentX, fragmentY, shot.direction)
			}
			if splitCount > 0 {
				g.spawnSplitActors(fragmentX, fragmentY, splitKind, splitCount, splitDirection)
			}
			if ricochet {
				shot.direction ^= 2
				shot.dx = -shot.dx
				shot.dy = -shot.dy
				kept = append(kept, shot)
			}
			continue
		}
		playerHit := false
		for playerIndex := range g.players {
			p := &g.players[playerIndex]
			if !p.active || p.hurtWait != 0 || playerIndex == shot.owner && shot.life > 6 || abs(shot.x-p.x) > 8 || abs(shot.y-p.y) > 8 {
				continue
			}
			g.damagePlayer(playerIndex)
			g.spawnTransientEntityState(12, shot.x, shot.y, shot.direction, 0, 6, 0, 0)
			playerHit = true
			break
		}
		if !playerHit {
			kept = append(kept, shot)
		}
	}
	g.bullets = kept
}

func (g *Game) transformEntityDeath(object *entity, direction int) {
	firstFrame := object.value
	speed := object.speed
	kind := 10 + g.deathToggle
	g.deathToggle ^= 1
	property, err := g.resources.EntityProperty(kind)
	if err != nil {
		object.active = false
		return
	}
	object.kind, object.direction, object.frame = kind, direction&3, 0
	object.speed, object.sequence = speed, int(property.Sequence)
	object.value, object.firstFrame = int(property.Value), firstFrame
	object.flags = property.Flags
	object.movement = bits.RotateLeft32(property.Movement, g.tick&31)
	object.graphics = 0x2ecd8 + uint32(property.Graphic)*original.TileSize
	object.vitality, object.life, object.age = 0, 8, 0
	object.successorKind, object.splitCount = int(property.Vitality&0xff), int(property.Vitality>>8)
	object.spawnWait, object.materialize, object.contactWait = 0, 0, 0
}

func (g *Game) promoteNextSegment(index int) {
	object := &g.entities[index]
	if object.flags&0x0c00 == 0 {
		return
	}
	g.script.setWord(4, g.script.word(4)+1)
	if index+1 >= len(g.entities) {
		return
	}
	next := &g.entities[index+1]
	if !next.active || next.flags&0x0800 == 0 {
		return
	}
	property, err := g.resources.EntityProperty(object.successorKind)
	if err != nil {
		return
	}
	next.kind = object.successorKind
	next.speed, next.sequence = int(property.Speed), int(property.Sequence)
	next.value, next.firstFrame = int(property.Value), -1
	next.flags, next.movement = property.Flags, g.runtimeMovement(next.kind, property)
	next.graphics = 0x2ecd8 + uint32(property.Graphic)*original.TileSize
	next.vitality = 0
	next.successorKind, next.splitCount = int(property.Vitality&0xff), int(property.Vitality>>8)
	if property.Flags&0x100 != 0 {
		next.vitality = int(property.Vitality)
	}
}

func (g *Game) spawnFragments(x, y, direction int) {
	for offset := range 4 {
		fragmentDirection := (direction + 2 + offset) & 3
		if fragmentDirection&1 == 0 && x%16 != 0 || fragmentDirection&1 != 0 && y%16 != 0 {
			continue
		}
		g.spawnTransientEntityState(2, x, y, fragmentDirection, 0, 10, 0, 0)
	}
}

func (g *Game) spawnSplitActors(x, y, kind, count, direction int) {
	spawnDirection := direction ^ 2
	for range count {
		if spawnDirection&1 == 0 && x%16 != 0 || spawnDirection&1 != 0 && y%16 != 0 {
			spawnDirection = (spawnDirection + 1) & 3
		}
		g.spawnTransientEntityState(kind, x, y, spawnDirection, 0, -1, 0, 0)
		spawnDirection = (spawnDirection + 1) & 3
	}
}

func (g *Game) damagePlayer(playerIndex int) {
	p := &g.players[playerIndex]
	if !p.active || p.hurtWait != 0 {
		return
	}
	g.playSound(8)
	p.hurtWait = 40
	if p.shadow {
		return
	}
	p.power--
	if p.power < 0 {
		p.power = 0
		p.key = false
		other := &g.players[playerIndex^1]
		if other.active && !other.shadow {
			p.shadow = true
		} else {
			p.active = false
			g.gameOver = gameOverDurationTicks
		}
	}
}

func (g *Game) projectilePassable(shot bullet) bool {
	x, y := (shot.x+8)/16, (shot.y+8)/16
	if x < 0 || y < 0 || x >= g.worldWidth || y >= g.worldHeight {
		return false
	}
	flags, tile := g.cell(x, y)
	collision, _ := g.resources.ResolveCell(g.worldIndex, g.worldPhase, flags, tile)
	if collision == 0x8e {
		g.setCell(x, y, flags, 0x93)
		g.playSound(2)
		return false
	}
	if collision < 0x92 || collision == 0xc9 || collision == 0xf4 {
		return false
	}
	return collision < 0xec || collision > 0xef || collision-0xec != byte(shot.direction)
}

func (g *Game) playSound(effect int) {
	if g.sounds != nil {
		g.sounds.Play(effect)
	}
}

func (g *Game) followPlayer() {
	viewWidth, viewHeight := ViewSize, ViewSize
	leadX, leadY := 80, 80
	if g.players[1].active {
		viewWidth, viewHeight = TwoViewWidth, TwoViewHeight
		leadX, leadY = 64, 48
	}
	for index := range g.players {
		p := &g.players[index]
		if !p.active {
			continue
		}
		g.cameraX[index] = max(0, min(g.worldWidth*16-viewWidth, p.x-leadX))
		g.cameraY[index] = max(0, min(g.worldHeight*16-viewHeight, p.y-leadY))
	}
}

// Draw renders the 320 by 200 original image between optional touch controls.
func (g *Game) Draw(screen *ebiten.Image) {
	target := screen
	if g.options.Touch {
		target = g.touchGame
	}
	target.Fill(color.Black)
	switch g.scene {
	case titleScene:
		phase := g.tick % palettePhases
		target.DrawImage(g.title[phase], nil)
		g.drawTitleControllers(target, phase)
	case heroesScene:
		g.drawHeroes(target)
	case heroDetailScene:
		g.drawHeroDetails(target)
	case heroNameScene:
		g.drawHeroName(target)
	default:
		g.drawPlay(target)
	}
	if g.dialog != nil {
		g.drawScriptDialog(target)
	}
	if g.options.Touch {
		screen.Fill(color.Black)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(TouchGameX, 0)
		screen.DrawImage(target, op)
		g.drawTouch(screen)
		if g.scene == heroNameScene {
			g.drawScriptKeyboard(screen)
		} else if g.dialog != nil && g.dialog.wantsInput && !(g.journal && g.dialog.maxLength == 1) {
			g.drawScriptKeyboard(screen)
		}
	}
	if g.capture {
		g.capture = false
		if g.options.Capture != "" {
			bounds := screen.Bounds()
			image := image.NewRGBA(bounds)
			screen.ReadPixels(image.Pix)
			file, err := os.Create(g.options.Capture)
			if err == nil {
				err = png.Encode(file, image)
				if closeErr := file.Close(); err == nil {
					err = closeErr
				}
			}
			g.err = err
		}
		g.done = true
	}
}

func (g *Game) drawTitleControllers(screen *ebiten.Image, phase int) {
	names := [...]string{" KEYBOARD ", "JOYSTICK 1", "JOYSTICK 2"}
	palette := g.resources.Palette(0, phase)
	for index, x := range [...]int{16, 176} {
		vector.DrawFilledRect(screen, float32(x), 139, 80, 10, color.Black, false)
		g.drawTextStyle(screen, names[g.controllers[index]], x, 139, 1, palette[2], palette[5])
	}
}

func (g *Game) drawPlay(screen *ebiten.Image) {
	phase := g.renderPalettePhase()
	if g.players[1].active {
		g.drawWorldView(screen, phase, 0, 0, 0, TwoViewWidth, TwoViewHeight)
		g.drawWorldView(screen, phase, 1, 176, 0, TwoViewWidth, TwoViewHeight)
		g.drawPlayerStatus(screen, 0, 0, 120, 0x46)
		g.drawPlayerStatus(screen, 1, 176, 120, 0x21)
	} else {
		g.drawWorldView(screen, phase, 0, 0, 0, ViewSize, ViewSize)
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(192, 0)
		screen.DrawImage(g.hud, op)
		screen.DrawImage(g.hudLogo[phase], op)
		vector.DrawFilledRect(screen, 256, 120, 64, 12, color.Black, false)
		g.drawWorldText(screen, fmt.Sprintf("%6d", g.players[0].score*10), 264, 122, 0x46)
		vector.DrawFilledRect(screen, 192, 140, 128, 12, color.Black, false)
		g.drawWorldText(screen, strings.Repeat("*", g.players[0].power), 192, 142, 0x46)
		vector.DrawFilledRect(screen, 192, 158, 128, 12, color.Black, false)
		if g.players[0].ratingSamples > 1 {
			g.drawWorldText(screen, g.ratingText(&g.players[0]), 192, 160, 0x46)
		}
	}
	g.drawWorldName(screen)
	if g.demo {
		prompt, x := `"Q" TO QUIT DEMO`, 192
		if g.options.Touch {
			prompt, x = `"TAP" TO QUIT DEMO`, 176
		}
		g.drawWorldText(screen, prompt, x, 184, 0x3f)
	}
	if g.paused {
		g.drawWorldText(screen, pausePrompt, 23*8, 188, 0xf5)
	}
	if g.gameOver > 0 && g.gameOver <= gameOverMessageTicks {
		g.drawWorldText(screen, gameOverPrompt, 0, 188, 0xff)
	}
}

func (g *Game) renderPalettePhase() int {
	phase := g.tick % palettePhases
	if g.powerPaletteActive() {
		phase += poweredPhaseBase
	}
	return phase
}

func (g *Game) powerPaletteActive() bool {
	return g.powerTimer > 25 || g.powerTimer > 0 && g.powerTimer&4 != 0
}

func (g *Game) drawWorldView(screen *ebiten.Image, phase, camera, destinationX, destinationY, viewWidth, viewHeight int) {
	g.world.Clear()
	firstX, firstY := g.cameraX[camera]/16, g.cameraY[camera]/16
	offsetX, offsetY := -(g.cameraX[camera] % 16), -(g.cameraY[camera] % 16)
	for y := 0; y <= viewHeight/16+1; y++ {
		for x := 0; x <= viewWidth/16+1; x++ {
			flags, tile := g.cell(firstX+x, firstY+y)
			atlas := g.tiles[phase]
			if flags&0x80 != 0 {
				atlas = g.composites[phase]
			}
			g.drawTile(g.world, atlas, int(tile), offsetX+x*16, offsetY+y*16)
		}
	}
	for _, object := range g.entities {
		if !object.active {
			continue
		}
		x, y := object.x-g.cameraX[camera], object.y-g.cameraY[camera]
		animation, _ := g.resources.EntityAnimation(object.sequence, object.direction, object.frame)
		if object.frame == 0 && object.firstFrame >= 0 {
			animation = object.firstFrame
		}
		graphic := object.graphics + uint32(animation*original.TileSize)
		if object.materialize > 0 {
			g.drawMaskedGraphic(g.world, phase, graphic, x, y, materializeMask(object.materialize))
		} else {
			g.drawGraphic(g.world, phase, graphic, x, y)
		}
	}
	for index := range g.players {
		p := &g.players[index]
		if !p.active {
			continue
		}
		if p.hurtWait != 0 && p.hurtWait&4 == 0 {
			continue
		}
		frame := banditAnimationFrame(p)
		if index == 1 {
			frame += 18
		}
		g.drawMaskedTile(g.world, g.bandits[phase], g.banditMasks, frame, p.x-g.cameraX[camera], p.y-g.cameraY[camera])
	}
	for _, shot := range g.bullets {
		property, _ := g.resources.EntityProperty(shot.owner)
		animation, _ := g.resources.EntityAnimation(int(property.Sequence), shot.direction, shot.frame)
		pointer := uint32(0x2ecd8) + uint32(property.Graphic+uint16(animation))*original.TileSize
		g.drawGraphic(g.world, phase, pointer, shot.x-g.cameraX[camera], shot.y-g.cameraY[camera])
	}
	view := g.world.SubImage(imageRect(0, 0, viewWidth, viewHeight)).(*ebiten.Image)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(destinationX), float64(destinationY))
	screen.DrawImage(view, op)
}

func (g *Game) drawPlayerStatus(screen *ebiten.Image, playerIndex, x, y int, textColor byte) {
	p := &g.players[playerIndex]
	g.drawWorldText(screen, "CUBITS:", x, y, textColor)
	g.drawWorldText(screen, fmt.Sprintf("%d", p.score*10), x+64, y, textColor)
	g.drawWorldText(screen, strings.Repeat("*", p.power), x, y+20, textColor)
	vector.StrokeRect(screen, float32(x+32), float32(y+44), 16, 16, 1, color.NRGBA{0, 102, 102, 255}, false)
	g.drawText(screen, "?", x+36, y+47, color.NRGBA{238, 0, 0, 255})
}

func (g *Game) drawWorldName(screen *ebiten.Image) {
	name := "THE TIMEGATES"
	if g.worldIndex < len(g.worldNames) {
		name = g.worldNames[g.worldIndex]
	}
	if len(name) > 18 {
		name = name[:18]
	}
	g.drawWorldText(screen, name, 0, 184, 0x37)
	if g.worldIndex == 0 {
		return
	}
	phaseName := fmt.Sprintf("(%d%c)", g.worldPhase/4+1, 'A'+rune(g.worldPhase%4))
	g.drawWorldText(screen, phaseName, (len(name)+1)*8, 184, 0x21)
}

func (g *Game) currentWorldPalette() color.Palette {
	palette := g.resources.Colors(g.paletteData, g.tick%palettePhases)
	if g.powerPaletteActive() {
		palette[15] = color.NRGBA{0, 0, 238, 255}
	}
	return palette
}

func (g *Game) drawWorldText(target *ebiten.Image, text string, x, y int, textColor byte) {
	palette := g.currentWorldPalette()
	g.drawTextStyle(target, text, x, y, 0, palette[textColor>>4], palette[textColor&0x0f])
}

func (g *Game) drawTile(target, atlas *ebiten.Image, index, x, y int) {
	columns := atlas.Bounds().Dx() / 16
	if index < 0 || index >= columns*(atlas.Bounds().Dy()/16) {
		return
	}
	source := atlas.SubImage(imageRect(index%columns*16, index/columns*16, 16, 16)).(*ebiten.Image)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	target.DrawImage(source, op)
}

func (g *Game) drawGraphic(target *ebiten.Image, phase int, pointer uint32, x, y int) {
	source, ok := g.graphicSource(phase, pointer)
	if !ok {
		return
	}
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	target.DrawImage(source, op)
}

func (g *Game) graphicSource(phase int, pointer uint32) (*ebiten.Image, bool) {
	var atlas *ebiten.Image
	index := 0
	switch {
	case pointer >= 0x2ecd8 && pointer < 0x2fed8:
		atlas, index = g.bandits[phase], int(pointer-0x2ecd8)/original.TileSize
	case pointer >= 0x2fed8 && pointer < 0x373d8:
		atlas, index = g.actors[phase], int(pointer-0x2fed8)/original.TileSize
	case pointer >= 0x373d8 && pointer < 0x380d8:
		atlas, index = g.icons[phase], int(pointer-0x373d8)/original.TileSize
	case pointer >= 0x20bd8 && pointer < 0x286d8:
		atlas, index = g.tiles[phase], int(pointer-0x20bd8)/original.TileSize
	default:
		return nil, false
	}
	columns := atlas.Bounds().Dx() / 16
	if index < 0 || index >= columns*(atlas.Bounds().Dy()/16) {
		return nil, false
	}
	return atlas.SubImage(imageRect(index%columns*16, index/columns*16, 16, 16)).(*ebiten.Image), true
}

func (g *Game) drawMaskedGraphic(target *ebiten.Image, phase int, pointer uint32, x, y, mask int) {
	source, ok := g.graphicSource(phase, pointer)
	if !ok || g.spawnMasks == nil || g.maskedActor == nil || mask < 0 || mask >= 8 {
		return
	}
	g.maskedActor.Clear()
	g.maskedActor.DrawImage(source, nil)
	maskSource := g.spawnMasks.SubImage(imageRect(mask*16, 0, 16, 16)).(*ebiten.Image)
	maskOptions := &ebiten.DrawImageOptions{Blend: ebiten.BlendDestinationIn}
	g.maskedActor.DrawImage(maskSource, maskOptions)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	target.DrawImage(g.maskedActor, op)
}

func (g *Game) drawMaskedTile(target, atlas, masks *ebiten.Image, index, x, y int) {
	if atlas == nil || masks == nil || g.maskedActor == nil || index < 0 {
		return
	}
	columns := atlas.Bounds().Dx() / 16
	if columns == 0 || index >= columns*(atlas.Bounds().Dy()/16) || index*16+16 > masks.Bounds().Dx() {
		return
	}
	source := atlas.SubImage(imageRect(index%columns*16, index/columns*16, 16, 16)).(*ebiten.Image)
	maskSource := masks.SubImage(imageRect(index*16, 0, 16, 16)).(*ebiten.Image)
	g.maskedActor.Clear()
	g.maskedActor.DrawImage(source, nil)
	g.maskedActor.DrawImage(maskSource, &ebiten.DrawImageOptions{Blend: ebiten.BlendDestinationIn})
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(float64(x), float64(y))
	target.DrawImage(g.maskedActor, op)
}

func materializeMask(remaining int) int {
	if remaining <= 0 {
		return 0
	}
	if remaining >= 21 {
		return 1
	}
	sequence := [...]byte{1, 2, 1, 2, 3, 2, 3, 4, 3, 4, 5, 4, 5, 6, 5, 6, 7, 6, 7, 0}
	return int(sequence[20-remaining])
}

func imageRect(x, y, width, height int) image.Rectangle { return image.Rect(x, y, x+width, y+height) }

func (g *Game) drawText(target *ebiten.Image, text string, x, y int, tint color.Color) {
	g.drawTextStyle(target, text, x, y, 0, tint, tint)
}

func (g *Game) drawTextStyle(target *ebiten.Image, text string, x, y, style int, first, second color.Color) {
	if style < 0 || style >= len(g.fonts) {
		return
	}
	colors := [2]color.Color{first, second}
	for _, character := range strings.ToUpper(text) {
		index := int(character) - 32
		if index < 0 || index >= 59 {
			x += 8
			continue
		}
		for layer, tint := range colors {
			r, green, b, a := tint.RGBA()
			source := g.fonts[style][layer].SubImage(imageRect(index%16*8, index/16*10, 8, 10)).(*ebiten.Image)
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(x), float64(y))
			op.ColorScale.Scale(float32(r)/65535, float32(green)/65535, float32(b)/65535, float32(a)/65535)
			target.DrawImage(source, op)
		}
		x += 8
	}
}

// Layout preserves exact pixels and exposes a side control panel on touch.
func (g *Game) Layout(_, _ int) (int, int) {
	if g.options.Touch {
		return TouchWidth, ScreenHeight
	}
	return ScreenWidth, ScreenHeight
}

// Pause is called by Android before the surface is suspended.
func (g *Game) Pause() { g.paused = true }

// Back applies the same contextual navigation as Escape.
func (g *Game) Back() {
	switch g.scene {
	case heroNameScene:
		g.acceptHeroName()
		return
	case heroDetailScene:
		g.scene = heroesScene
		return
	case heroesScene:
		g.scene, g.idle = titleScene, 0
		return
	}
	if g.scene == playScene {
		if g.paused {
			g.scene = titleScene
			g.paused = false
		} else {
			g.paused = true
		}
	}
}

func (g *Game) readTouch() {
	g.touchFire = false
	stickActive := false
	for _, id := range ebiten.AppendTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		if touchPauseContains(x, y) {
			continue
		}
		if x < TouchGameX && !stickActive {
			dx, dy := float64(x-TouchStickX)/38, float64(y-TouchControlY)/38
			length := math.Hypot(dx, dy)
			if length > 1 {
				dx, dy = dx/length, dy/length
			}
			g.touchDirection = cardinalTouchDirection(dx, dy, g.touchDirection)
			stickActive = true
		} else if x >= TouchRightX && y > 82 {
			g.touchFire = true
		}
	}
	if !stickActive {
		g.touchDirection = -1
	}
	if g.pauseTouchPressed() {
		g.paused = true
	}
}

func cardinalTouchDirection(dx, dy float64, previous int) int {
	const (
		deadZone   = 0.28
		hysteresis = 1.35
	)
	if math.Hypot(dx, dy) < deadZone {
		return -1
	}
	ax, ay := math.Abs(dx), math.Abs(dy)
	switch previous {
	case 0:
		if dy < 0 && ax <= ay*hysteresis {
			return previous
		}
	case 1:
		if dx > 0 && ay <= ax*hysteresis {
			return previous
		}
	case 2:
		if dy > 0 && ax <= ay*hysteresis {
			return previous
		}
	case 3:
		if dx < 0 && ay <= ax*hysteresis {
			return previous
		}
	}
	if ax > ay {
		if dx > 0 {
			return 1
		}
		return 3
	}
	if dy > 0 {
		return 2
	}
	return 0
}

func touchDirectionVector(direction int) (int, int) {
	if direction < 0 || direction >= 4 {
		return 0, 0
	}
	vector := [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}[direction]
	return vector[0], vector[1]
}

func (g *Game) pauseTouchPressed() bool {
	for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
		x, y := ebiten.TouchPosition(id)
		if touchPauseContains(x, y) {
			return true
		}
	}
	return false
}

func touchPauseContains(x, y int) bool {
	return x >= TouchPauseX && x < TouchPauseX+TouchPauseWidth && y >= TouchPauseY && y < TouchPauseY+TouchPauseHeight
}

func (g *Game) drawTouch(screen *ebiten.Image) {
	panel := color.NRGBA{8, 18, 20, 255}
	vector.DrawFilledRect(screen, 0, 0, TouchGameX, ScreenHeight, panel, false)
	vector.DrawFilledRect(screen, TouchRightX, 0, TouchWidth-TouchRightX, ScreenHeight, panel, false)
	if g.dialog != nil {
		return
	}
	if g.scene == titleScene {
		vector.DrawFilledRect(screen, 8, 8, TouchGameX-16, 48, color.NRGBA{34, 70, 72, 255}, false)
		g.drawText(screen, "LORDS", 28, 27, color.White)
		vector.DrawFilledRect(screen, 8, 76, TouchGameX-16, 48, color.NRGBA{34, 70, 72, 255}, false)
		g.drawText(screen, "1 PLAYER", 16, 95, color.White)
		vector.DrawFilledRect(screen, TouchRightX+4, 76, TouchWidth-TouchRightX-8, 48, color.NRGBA{34, 70, 72, 255}, false)
		g.drawText(screen, "2P", TouchRightX+24, 95, color.White)
		return
	}
	if g.scene == heroNameScene {
		return
	}
	if g.scene != playScene {
		g.drawText(screen, "BACK", 32, 16, color.White)
		return
	}
	guide := color.NRGBA{0, 102, 102, 220}
	vector.StrokeCircle(screen, TouchStickX, TouchControlY, 42, 3, color.NRGBA{0, 238, 238, 220}, false)
	vector.StrokeLine(screen, TouchStickX-34, TouchControlY, TouchStickX+34, TouchControlY, 2, guide, false)
	vector.StrokeLine(screen, TouchStickX, TouchControlY-34, TouchStickX, TouchControlY+34, 2, guide, false)
	dx, dy := touchDirectionVector(g.touchDirection)
	vector.DrawFilledCircle(screen, float32(TouchStickX+dx*24), float32(TouchControlY+dy*24), 17, color.NRGBA{0, 136, 136, 220}, false)
	vector.DrawFilledCircle(screen, TouchFireX, TouchControlY, 28, color.NRGBA{204, 0, 0, 230}, false)
	g.drawText(screen, "FIRE", TouchFireX-16, TouchControlY-4, color.White)
	vector.DrawFilledRect(screen, TouchPauseX, TouchPauseY, TouchPauseWidth, TouchPauseHeight, color.NRGBA{34, 70, 72, 255}, false)
	g.drawText(screen, "PAUSE", TouchPauseX+8, TouchPauseY+9, color.White)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
