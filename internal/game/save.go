package game

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/olivierh59500/timebandits/internal/original"
)

const saveFormatVersion = 3

type saveDocument struct {
	Version   int                   `json:"version"`
	Slots     [2]*savedGame         `json:"slots"`
	TimeLords [heroCount]heroRecord `json:"time_lords"`
}

type savedGame struct {
	Tick         int                        `json:"tick"`
	WorldIndex   int                        `json:"world_index"`
	WorldPhase   int                        `json:"world_phase"`
	WorldPhases  [original.WorldCount]uint8 `json:"world_phases"`
	WorldJournal [original.WorldCount]uint8 `json:"world_journal"`
	WorldDone    [original.WorldCount]bool  `json:"world_done"`
	WorldVisited [original.WorldCount]bool  `json:"world_visited"`
	Players      [2]savedPlayer             `json:"players"`
	MapData      []byte                     `json:"map_data"`
	Bullets      []savedBullet              `json:"bullets"`
	Entities     []savedEntity              `json:"entities"`
	Treasure     int                        `json:"treasure"`
	Pellets      int                        `json:"pellets"`
	PowerTimer   int                        `json:"power_timer"`
	PowerCombo   int                        `json:"power_combo"`
	Random       uint32                     `json:"random"`
	SpawnCursor  int                        `json:"spawn_cursor"`
	DeathToggle  int                        `json:"death_toggle"`
	ScriptState  []byte                     `json:"script_state"`
	PaletteData  []byte                     `json:"palette_data"`
}

type savedPlayer struct {
	X             int        `json:"x"`
	Y             int        `json:"y"`
	Direction     int        `json:"direction"`
	Frame         int        `json:"frame"`
	Power         int        `json:"power"`
	Rating        int        `json:"rating"`
	RatingScore   int        `json:"rating_score"`
	KillBonus     int        `json:"kill_bonus"`
	RatingWait    int        `json:"rating_wait"`
	RatingTotal   uint64     `json:"rating_total"`
	RatingSamples uint32     `json:"rating_samples"`
	TrailCursor   int        `json:"trail_cursor"`
	TrailPosition [32]uint16 `json:"trail_position"`
	TrailVisits   [32]uint16 `json:"trail_visits"`
	Score         int        `json:"score"`
	ScorePending  int        `json:"score_pending"`
	Key           bool       `json:"key"`
	FireWait      int        `json:"fire_wait"`
	HurtWait      int        `json:"hurt_wait"`
	ScriptWait    int        `json:"script_wait"`
	Shadow        bool       `json:"shadow"`
	Active        bool       `json:"active"`
}

type savedBullet struct {
	X         int  `json:"x"`
	Y         int  `json:"y"`
	DX        int  `json:"dx"`
	DY        int  `json:"dy"`
	Owner     int  `json:"owner"`
	Direction int  `json:"direction"`
	Frame     int  `json:"frame"`
	Life      int  `json:"life"`
	Active    bool `json:"active"`
}

type savedEntity struct {
	X                 int    `json:"x"`
	Y                 int    `json:"y"`
	Kind              int    `json:"kind"`
	Direction         int    `json:"direction"`
	Frame             int    `json:"frame"`
	Speed             int    `json:"speed"`
	Sequence          int    `json:"sequence"`
	Vitality          int    `json:"vitality"`
	Value             int    `json:"value"`
	FirstFrame        int    `json:"first_frame"`
	PreviousDirection int    `json:"previous_direction"`
	SuccessorKind     int    `json:"successor_kind"`
	SplitCount        int    `json:"split_count"`
	Flags             uint16 `json:"flags"`
	Movement          uint32 `json:"movement"`
	Graphics          uint32 `json:"graphics"`
	Age               int    `json:"age"`
	Life              int    `json:"life"`
	SpawnWait         int    `json:"spawn_wait"`
	Materialize       int    `json:"materialize"`
	ContactWait       int    `json:"contact_wait"`
	FireWait          int    `json:"fire_wait"`
	Active            bool   `json:"active"`
}

func defaultSavePath() string {
	directory, err := os.UserConfigDir()
	if err != nil || directory == "" {
		return ""
	}
	return filepath.Join(directory, "timebandits-go", "saves.json")
}

func (g *Game) scriptSaveSlot(variable int) (int, bool) {
	value := strings.TrimSpace(g.script.stringValue(variable))
	if len(value) == 0 || value[0] < '1' || value[0] > '2' {
		return 0, false
	}
	return int(value[0] - '1'), true
}

func (g *Game) saveSlot(slot int) error {
	if slot < 0 || slot >= len(g.saveSlots) {
		return fmt.Errorf("save slot %d is outside the journal", slot+1)
	}
	if err := g.ensureSavesLoaded(); err != nil {
		return err
	}
	g.saveSlots[slot] = g.captureSavedGame()
	return g.persistSaveSlots()
}

func (g *Game) restoreSlot(slot int) (bool, error) {
	if slot < 0 || slot >= len(g.saveSlots) {
		return false, fmt.Errorf("save slot %d is outside the journal", slot+1)
	}
	if err := g.ensureSavesLoaded(); err != nil {
		return false, err
	}
	saved := g.saveSlots[slot]
	if saved == nil {
		return false, nil
	}
	if err := g.applySavedGame(saved); err != nil {
		return false, err
	}
	return true, nil
}

func (g *Game) ensureSavesLoaded() error {
	if g.savesLoaded {
		return nil
	}
	if g.savePath == "" {
		g.timeLords = defaultTimeLords()
		g.savesLoaded = true
		return nil
	}
	data, err := os.ReadFile(g.savePath)
	if errors.Is(err, os.ErrNotExist) {
		g.timeLords = defaultTimeLords()
		g.savesLoaded = true
		return nil
	}
	if err != nil {
		return err
	}
	var document saveDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode Time Bandits saves: %w", err)
	}
	if document.Version == 1 {
		migrateRatingState(&document)
	}
	if document.Version == 2 {
		document.TimeLords = defaultTimeLords()
		document.Version = saveFormatVersion
	} else if document.Version != saveFormatVersion {
		return fmt.Errorf("unsupported Time Bandits save version %d", document.Version)
	}
	for slot, saved := range document.Slots {
		if saved != nil {
			if err := validateSavedGame(saved); err != nil {
				return fmt.Errorf("invalid Time Bandits save slot %d: %w", slot+1, err)
			}
		}
	}
	if err := validateHeroes(document.TimeLords); err != nil {
		return fmt.Errorf("invalid Time Bandits Timelords: %w", err)
	}
	g.saveSlots = document.Slots
	g.timeLords = document.TimeLords
	g.savesLoaded = true
	return nil
}

func migrateRatingState(document *saveDocument) {
	for _, saved := range document.Slots {
		if saved == nil {
			continue
		}
		for index := range saved.Players {
			player := &saved.Players[index]
			player.Rating, player.RatingScore, player.KillBonus = 5, 450, 2
			player.RatingWait, player.RatingSamples = 99, 1
			player.TrailCursor = len(player.TrailPosition) - 1
			for trail := range player.TrailVisits {
				player.TrailVisits[trail] = 16
			}
		}
	}
	document.TimeLords = defaultTimeLords()
	document.Version = saveFormatVersion
}

func (g *Game) persistSaveSlots() error {
	if g.savePath == "" {
		return nil
	}
	data, err := json.MarshalIndent(saveDocument{Version: saveFormatVersion, Slots: g.saveSlots, TimeLords: g.timeLords}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(g.savePath), 0700); err != nil {
		return err
	}
	temporary := g.savePath + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(temporary, g.savePath); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}

func (g *Game) captureSavedGame() *savedGame {
	saved := &savedGame{
		Tick: g.tick, WorldIndex: g.worldIndex, WorldPhase: g.worldPhase,
		WorldPhases: g.worldPhases, WorldJournal: g.worldJournal,
		WorldDone: g.worldDone, WorldVisited: g.worldVisited,
		MapData: append([]byte(nil), g.mapData...), Treasure: g.treasure,
		Pellets: g.pellets, PowerTimer: g.powerTimer, PowerCombo: g.powerCombo, Random: g.random,
		SpawnCursor: g.spawnCursor, DeathToggle: g.deathToggle,
		ScriptState: append([]byte(nil), g.script.state...), PaletteData: append([]byte(nil), g.paletteData...),
	}
	for index, player := range g.players {
		saved.Players[index] = savedPlayer{
			X: player.x, Y: player.y, Direction: player.direction, Frame: player.frame,
			Power: player.power, Rating: player.rating, RatingScore: player.ratingScore,
			KillBonus: player.killBonus, RatingWait: player.ratingWait,
			RatingTotal: player.ratingTotal, RatingSamples: player.ratingSamples,
			TrailCursor: player.trailCursor, TrailPosition: player.trailPosition, TrailVisits: player.trailVisits,
			Score: player.score, ScorePending: player.scorePending,
			Key: player.key, FireWait: player.fireWait, HurtWait: player.hurtWait,
			ScriptWait: player.scriptWait, Shadow: player.shadow, Active: player.active,
		}
	}
	saved.Bullets = make([]savedBullet, len(g.bullets))
	for index, shot := range g.bullets {
		saved.Bullets[index] = savedBullet{
			X: shot.x, Y: shot.y, DX: shot.dx, DY: shot.dy, Owner: shot.owner,
			Direction: shot.direction, Frame: shot.frame, Life: shot.life, Active: shot.active,
		}
	}
	saved.Entities = make([]savedEntity, len(g.entities))
	for index, object := range g.entities {
		saved.Entities[index] = savedEntity{
			X: object.x, Y: object.y, Kind: object.kind, Direction: object.direction, Frame: object.frame,
			Speed: object.speed, Sequence: object.sequence, Vitality: object.vitality,
			Value: object.value, FirstFrame: object.firstFrame,
			PreviousDirection: object.previousDirection, SuccessorKind: object.successorKind, SplitCount: object.splitCount,
			Flags: object.flags, Movement: object.movement, Graphics: object.graphics,
			Age: object.age, Life: object.life, SpawnWait: object.spawnWait,
			Materialize: object.materialize, ContactWait: object.contactWait, FireWait: object.fireWait, Active: object.active,
		}
	}
	return saved
}

func validateSavedGame(saved *savedGame) error {
	if saved.WorldIndex < 0 || saved.WorldIndex >= original.WorldCount {
		return fmt.Errorf("world %d is outside recovered data", saved.WorldIndex)
	}
	if saved.WorldPhase < 0 || saved.WorldPhase >= original.PhaseCount || int(saved.WorldPhases[saved.WorldIndex]) != saved.WorldPhase {
		return fmt.Errorf("phase %d does not match world state", saved.WorldPhase)
	}
	if len(saved.MapData) != original.MapCapacity || len(saved.ScriptState) != original.ScriptStateSize || len(saved.PaletteData) != original.RuntimePaletteSize {
		return fmt.Errorf("saved runtime buffers have invalid lengths")
	}
	if len(saved.Entities) > original.EntityCount || len(saved.Bullets) > 4 {
		return fmt.Errorf("saved actor counts are invalid")
	}
	for index, shot := range saved.Bullets {
		if shot.Owner < 0 || shot.Owner >= len(saved.Players) || shot.Life < 0 || shot.Life > 12 {
			return fmt.Errorf("saved projectile %d is invalid", index+1)
		}
	}
	if saved.Pellets < 0 || saved.Pellets > 181 || saved.PowerTimer < 0 || saved.PowerTimer > 130 || saved.PowerCombo < 0 || saved.PowerCombo > 4 {
		return fmt.Errorf("saved collectible state is invalid")
	}
	for index, player := range saved.Players {
		if player.Rating < 0 || player.Rating >= original.RatingCount || player.TrailCursor < 0 || player.TrailCursor >= len(player.TrailPosition) {
			return fmt.Errorf("saved player %d rating history is invalid", index+1)
		}
	}
	return nil
}

func (g *Game) applySavedGame(saved *savedGame) error {
	if err := validateSavedGame(saved); err != nil {
		return err
	}
	g.worldPhases, g.worldJournal = saved.WorldPhases, saved.WorldJournal
	g.worldDone, g.worldVisited = saved.WorldDone, saved.WorldVisited
	if err := g.loadWorld(saved.WorldIndex); err != nil {
		return err
	}
	g.tick, g.treasure, g.random = saved.Tick, saved.Treasure, saved.Random
	g.pellets, g.powerTimer, g.powerCombo = saved.Pellets, saved.PowerTimer, saved.PowerCombo
	g.spawnCursor, g.deathToggle = saved.SpawnCursor, saved.DeathToggle
	g.mapData = append(g.mapData[:0], saved.MapData...)
	g.script.state = append(g.script.state[:0], saved.ScriptState...)
	g.script.message.Reset()
	g.paletteData = append(g.paletteData[:0], saved.PaletteData...)
	g.paletteDirty = false
	if err := g.buildWorldAtlases(g.worldIndex, g.worldPhase); err != nil {
		return err
	}
	for index, savedPlayer := range saved.Players {
		g.players[index] = player{
			x: savedPlayer.X, y: savedPlayer.Y, direction: savedPlayer.Direction, frame: savedPlayer.Frame,
			power: savedPlayer.Power, rating: savedPlayer.Rating, ratingScore: savedPlayer.RatingScore,
			killBonus: savedPlayer.KillBonus, ratingWait: savedPlayer.RatingWait,
			ratingTotal: savedPlayer.RatingTotal, ratingSamples: savedPlayer.RatingSamples,
			trailCursor: savedPlayer.TrailCursor, trailPosition: savedPlayer.TrailPosition, trailVisits: savedPlayer.TrailVisits,
			score: savedPlayer.Score, scorePending: savedPlayer.ScorePending,
			key: savedPlayer.Key, fireWait: savedPlayer.FireWait, hurtWait: savedPlayer.HurtWait,
			scriptWait: savedPlayer.ScriptWait, shadow: savedPlayer.Shadow, active: savedPlayer.Active,
		}
	}
	g.bullets = make([]bullet, len(saved.Bullets))
	for index, shot := range saved.Bullets {
		g.bullets[index] = bullet{
			x: shot.X, y: shot.Y, dx: shot.DX, dy: shot.DY, owner: shot.Owner,
			direction: shot.Direction, frame: shot.Frame, life: shot.Life, active: shot.Active,
		}
	}
	g.entities = make([]entity, len(saved.Entities))
	for index, object := range saved.Entities {
		successorKind, splitCount := object.SuccessorKind, object.SplitCount
		if successorKind == 0 && splitCount == 0 {
			if property, err := g.resources.EntityProperty(object.Kind); err == nil {
				successorKind, splitCount = int(property.Vitality&0xff), int(property.Vitality>>8)
			}
		}
		firstFrame := object.FirstFrame
		if firstFrame == 0 && object.Kind != 10 && object.Kind != 11 {
			firstFrame = -1
		}
		g.entities[index] = entity{
			x: object.X, y: object.Y, kind: object.Kind, direction: object.Direction, frame: object.Frame,
			speed: object.Speed, sequence: object.Sequence, vitality: object.Vitality,
			value: object.Value, firstFrame: firstFrame,
			previousDirection: object.PreviousDirection, successorKind: successorKind, splitCount: splitCount,
			flags: object.Flags, movement: object.Movement, graphics: object.Graphics,
			age: object.Age, life: object.Life, spawnWait: object.SpawnWait,
			materialize: object.Materialize, contactWait: object.ContactWait, fireWait: object.FireWait, active: object.Active,
		}
	}
	if len(g.entities) < original.EntityCount {
		g.entities = append(g.entities, make([]entity, original.EntityCount-len(g.entities))...)
	}
	g.scene, g.demo, g.paused, g.gameOver = playScene, false, false, 0
	g.dialog, g.journal = nil, false
	g.touchDirection, g.touchFire = -1, false
	g.followPlayer()
	return nil
}
