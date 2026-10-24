package original

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"strings"

	gameassets "github.com/olivierh59500/timebandits/assets"
)

// DiskSHA256 identifies the original disk revision represented by the bundled resources.
const DiskSHA256 = "f70ac94e2820cbac5255aa1b3b18b44c090b0464d9fcda50cc2cae5dedc07689"

const (
	TileCount              = 246
	ActorCount             = 234
	BanditCount            = 36
	TileSize               = 128
	MapStride              = 128
	MapCapacity            = 64 * 64 * 2
	EntityCount            = 63
	EntitySize             = 72
	RemapSize              = 0x32
	WorldAuxSize           = 0x2a6
	CompositeTileCount     = 25
	CompositeTileSize      = CompositeTileCount * TileSize
	ArtifactCount          = 8
	IconCount              = 26
	RatingCount            = 10
	RatingDataSize         = 0x110
	RuntimePaletteSize     = 32
	TitleLogoWidth         = 288
	TitleLogoHeight        = 84
	HUDLogoWidth           = 112
	HUDLogoHeight          = 113
	TimeLordsLogoWidth     = 48
	TimeLordsLogoHeight    = 71
	TodaysHeroesLogoWidth  = 80
	TodaysHeroesLogoHeight = 71
	ScriptStateSize        = 0x480
	WorldLogicBase         = 0x100d2
	WorldLogicSize         = 0x20bd8 - WorldLogicBase
	WorldCount             = 17
	PhaseCount             = 16
	portalOffset           = 0
	exitOffset             = 0x80
	spawnOffset            = 0x224
)

// Resources holds immutable source-derived bytes and decoded reference art.
type Resources struct {
	Palettes, WorldTables, TileProperties     []byte
	EntityAnimations                          []byte
	Tiles, Actors, BanditOne, BanditTwo       []byte
	Artifacts                                 []byte
	SpawnMasks                                []byte
	RatingData                                []byte
	Icons, Fonts, Timegates                   []byte
	TitleLogo, HUDLogo                        []byte
	TimeLordsLogo, TodaysHeroesLogo           []byte
	Maps, Entities, Remaps, WorldAux          []byte
	CompositeTiles                            []byte
	RuntimePalettes                           []byte
	ScriptStates, ScriptPreStates, WorldLogic []byte
	Effects                                   [10][]byte
	Worlds                                    []World
	Title, HUD                                image.Image
}

// World is one independently initialized original map descriptor.
type World struct {
	World, Phase                       int
	Random                             uint32
	Width, Height                      int
	PlayerX, PlayerY, CameraX, CameraY int
	PortalCount, ExitCount, SpawnCount int
	MapSHA256, EntitiesSHA256          string
	RemapSHA256, AuxSHA256             string
	CompositeSHA256                    string
	PaletteSHA256                      string
	ScriptStateSHA256                  string
	ScriptPreSHA256                    string
}

// Portal is one phase-adjusted map teleporter destination in tile units.
type Portal struct{ X, Y int }

// Exit is one world transition and the two destination spawn positions.
type Exit struct {
	World                  int
	OneX, OneY, TwoX, TwoY int
}

// EntityProperty is one original 16-byte actor behavior descriptor.
type EntityProperty struct {
	Graphic, Value, Sequence, Speed uint16
	Flags                           uint16
	Movement                        uint32
	Vitality                        uint16
}

// RatingProfile is one original adaptive play-style rank.
type RatingProfile struct {
	Name                 string
	Threshold, KillBonus int
	Drift                int
}

type manifest struct {
	DiskSHA256 string `json:"disk_sha256"`
	Files      map[string]struct {
		Bytes  int    `json:"bytes"`
		SHA256 string `json:"sha256"`
	} `json:"files"`
}

// Load verifies resource lengths and decodes the two recovered static panels.
func Load() (*Resources, error) {
	read := func(name string) ([]byte, error) {
		return fs.ReadFile(gameassets.Original, "original/"+name)
	}
	result := &Resources{}
	var err error
	for name, target := range map[string]*[]byte{
		"palettes.bin": &result.Palettes, "world_tables.bin": &result.WorldTables,
		"rating_data.bin":       &result.RatingData,
		"entity_animations.bin": &result.EntityAnimations,
		"tile_properties.bin":   &result.TileProperties, "tiles.bin": &result.Tiles,
		"artifacts.bin": &result.Artifacts, "spawn_masks.bin": &result.SpawnMasks,
		"actors.bin": &result.Actors, "bandit_one.bin": &result.BanditOne,
		"bandit_two.bin": &result.BanditTwo, "icons.bin": &result.Icons,
		"fonts.bin": &result.Fonts, "timegates.map": &result.Timegates,
		"title_logo.bin": &result.TitleLogo, "hud_logo.bin": &result.HUDLogo,
		"time_lords_logo.bin": &result.TimeLordsLogo, "todays_heroes_logo.bin": &result.TodaysHeroesLogo,
		"maps.bin": &result.Maps, "entities.bin": &result.Entities,
		"remaps.bin": &result.Remaps, "world_aux.bin": &result.WorldAux,
		"composite_tiles.bin":  &result.CompositeTiles,
		"runtime_palettes.bin": &result.RuntimePalettes,
		"script_state.bin":     &result.ScriptStates, "script_pre_state.bin": &result.ScriptPreStates,
		"world_logic.bin": &result.WorldLogic,
	} {
		*target, err = read(name)
		if err != nil {
			return nil, err
		}
	}
	for effect := range result.Effects {
		result.Effects[effect], err = read(fmt.Sprintf("effect-%02d.pcm", effect))
		if err != nil || len(result.Effects[effect])%4 != 0 {
			return nil, fmt.Errorf("invalid recovered sound effect %d", effect)
		}
	}
	decode := func(name string) (image.Image, error) {
		file, err := gameassets.Original.Open("original/" + name)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		return png.Decode(file)
	}
	if result.Title, err = decode("title.png"); err != nil {
		return nil, err
	}
	if result.HUD, err = decode("hud.png"); err != nil {
		return nil, err
	}
	if len(result.Tiles) != TileCount*TileSize || len(result.Actors) != ActorCount*TileSize || len(result.BanditOne) != BanditCount*32 || len(result.BanditTwo) != BanditCount*TileSize || len(result.Icons) != IconCount*TileSize || len(result.Artifacts) != ArtifactCount*TileSize || len(result.SpawnMasks) != 8*32 || len(result.RatingData) != RatingDataSize || len(result.Timegates) != MapCapacity || len(result.TitleLogo) != TitleLogoWidth/16*TitleLogoHeight*8 || len(result.HUDLogo) != HUDLogoWidth/16*HUDLogoHeight*8 || len(result.TimeLordsLogo) != TimeLordsLogoWidth/16*TimeLordsLogoHeight*8 || len(result.TodaysHeroesLogo) != TodaysHeroesLogoWidth/16*TodaysHeroesLogoHeight*8 {
		return nil, fmt.Errorf("invalid recovered resource lengths")
	}
	worldData, err := read("worlds.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(worldData, &result.Worlds); err != nil || len(result.Worlds) != WorldCount*PhaseCount || len(result.Maps) != WorldCount*PhaseCount*MapCapacity || len(result.Entities) != WorldCount*PhaseCount*EntityCount*EntitySize || len(result.Remaps) != WorldCount*PhaseCount*RemapSize || len(result.WorldAux) != WorldCount*PhaseCount*WorldAuxSize || len(result.CompositeTiles) != WorldCount*PhaseCount*CompositeTileSize || len(result.RuntimePalettes) != WorldCount*PhaseCount*RuntimePaletteSize || len(result.ScriptStates) != WorldCount*PhaseCount*ScriptStateSize || len(result.ScriptPreStates) != WorldCount*PhaseCount*ScriptStateSize || len(result.WorldLogic) != WorldLogicSize {
		return nil, fmt.Errorf("invalid recovered world maps")
	}
	manifestData, err := read("manifest.json")
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(manifestData, &m); err != nil || m.DiskSHA256 != DiskSHA256 {
		return nil, fmt.Errorf("invalid recovered resource manifest")
	}
	for name, expected := range m.Files {
		data, err := read(name)
		if err != nil || len(data) != expected.Bytes || fmt.Sprintf("%x", sha256.Sum256(data)) != expected.SHA256 {
			return nil, fmt.Errorf("recovered resource checksum mismatch: %s", name)
		}
	}
	return result, nil
}

// Palette resolves one of the original twelve-frame color cycles.
func (r *Resources) Palette(index, phase int) color.Palette {
	start := index * RuntimePaletteSize
	return r.decodePalette(r.Palettes[start:start+RuntimePaletteSize], phase)
}

// WorldColors resolves the runtime palette after the world's startup script.
func (r *Resources) WorldColors(world, worldPhase, animationPhase int) color.Palette {
	index, ok := worldVariant(world, worldPhase)
	if !ok {
		return make(color.Palette, 16)
	}
	start := index * RuntimePaletteSize
	return r.Colors(r.RuntimePalettes[start:start+RuntimePaletteSize], animationPhase)
}

// Colors resolves a mutable runtime palette and its animated entries.
func (r *Resources) Colors(raw []byte, animationPhase int) color.Palette {
	if len(raw) < RuntimePaletteSize {
		return make(color.Palette, 16)
	}
	return r.decodePalette(raw[:RuntimePaletteSize], animationPhase)
}

func (r *Resources) decodePalette(raw []byte, phase int) color.Palette {
	palette := make(color.Palette, 16)
	phase = (phase % 12) * 2
	for entry := range palette {
		value := int16(binary.BigEndian.Uint16(raw[entry*2:]))
		if value < 0 {
			animated := 0x1a8 + int(-value)*24 + phase
			value = int16(binary.BigEndian.Uint16(r.Palettes[animated:]))
		}
		word := uint16(value) * 2
		palette[entry] = color.NRGBA{uint8(word>>8&15) * 17, uint8(word>>4&15) * 17, uint8(word&15) * 17, 255}
	}
	return palette
}

// DecodeTiles expands interleaved four-plane 16 by 16 cells into one atlas.
func DecodeTiles(raw []byte, palette color.Palette) (*image.Paletted, error) {
	if len(raw)%TileSize != 0 {
		return nil, fmt.Errorf("tile bank length %d is not aligned", len(raw))
	}
	count := len(raw) / TileSize
	columns := min(16, count)
	rows := (count + columns - 1) / columns
	atlas := image.NewPaletted(image.Rect(0, 0, columns*16, rows*16), palette)
	for tile := range count {
		left, top := tile%columns*16, tile/columns*16
		for y := range 16 {
			for x := range 16 {
				var index uint8
				for plane := range 4 {
					word := binary.BigEndian.Uint16(raw[tile*TileSize+y*8+plane*2:])
					index |= uint8(word>>uint(15-x)&1) << uint(plane)
				}
				atlas.SetColorIndex(left+x, top+y, index)
			}
		}
	}
	return atlas, nil
}

// DecodePlanarGraphic expands one interleaved four-plane rectangular picture.
func DecodePlanarGraphic(raw []byte, width, height int, palette color.Palette) (*image.Paletted, error) {
	if width <= 0 || width%16 != 0 || height <= 0 || len(raw) != width/16*height*8 {
		return nil, fmt.Errorf("planar graphic dimensions %dx%d do not match %d bytes", width, height, len(raw))
	}
	result := image.NewPaletted(image.Rect(0, 0, width, height), palette)
	groups := width / 16
	for y := range height {
		for group := range groups {
			offset := (y*groups + group) * 8
			for x := range 16 {
				var index uint8
				for plane := range 4 {
					word := binary.BigEndian.Uint16(raw[offset+plane*2:])
					index |= uint8(word>>uint(15-x)&1) << uint(plane)
				}
				result.SetColorIndex(group*16+x, y, index)
			}
		}
	}
	return result, nil
}

// DecodeMasks expands packed one-plane 16 by 16 materialization masks.
func DecodeMasks(raw []byte) (*image.Alpha, error) {
	const maskSize = 16 * 2
	if len(raw)%maskSize != 0 {
		return nil, fmt.Errorf("mask bank length %d is not aligned", len(raw))
	}
	count := len(raw) / maskSize
	atlas := image.NewAlpha(image.Rect(0, 0, count*16, 16))
	for mask := range count {
		for y := range 16 {
			word := binary.BigEndian.Uint16(raw[mask*maskSize+y*2:])
			for x := range 16 {
				if word&(1<<uint(15-x)) != 0 {
					atlas.SetAlpha(mask*16+x, y, color.Alpha{A: 255})
				}
			}
		}
	}
	return atlas, nil
}

// DecodeInverseMasks expands player masks whose set bits mark transparent pixels.
func DecodeInverseMasks(raw []byte) (*image.Alpha, error) {
	const maskSize = 16 * 2
	if len(raw)%maskSize != 0 {
		return nil, fmt.Errorf("inverse mask bank length %d is not aligned", len(raw))
	}
	count := len(raw) / maskSize
	atlas := image.NewAlpha(image.Rect(0, 0, count*16, 16))
	for mask := range count {
		for y := range 16 {
			word := binary.BigEndian.Uint16(raw[mask*maskSize+y*2:])
			for x := range 16 {
				if word&(1<<uint(15-x)) == 0 {
					atlas.SetAlpha(mask*16+x, y, color.Alpha{A: 255})
				}
			}
		}
	}
	return atlas, nil
}

// Cell returns the original flags and tile index at a 64-cell-stride position.
func (r *Resources) Cell(x, y int) (flags, tile byte) {
	return cell(r.Timegates, x, y)
}

// World returns the original initialized descriptor for one world phase.
func (r *Resources) World(world, phase int) (World, error) {
	index, ok := worldVariant(world, phase)
	if !ok {
		return World{}, fmt.Errorf("world %d phase %d is outside recovered data", world, phase)
	}
	return r.Worlds[index], nil
}

// WorldCell returns one initialized world phase's native flags and tile index.
func (r *Resources) WorldCell(world, phase, x, y int) (flags, tile byte) {
	data := r.WorldMap(world, phase)
	if data == nil {
		return 0xff, 0
	}
	return cell(data, x, y)
}

// WorldMap returns one immutable initialized 64 by 64 cell buffer.
func (r *Resources) WorldMap(world, phase int) []byte {
	index, ok := worldVariant(world, phase)
	if !ok {
		return nil
	}
	return r.Maps[index*MapCapacity : (index+1)*MapCapacity]
}

// WorldEntities returns the initialized 63-slot actor table for one world phase.
func (r *Resources) WorldEntities(world, phase int) []byte {
	index, ok := worldVariant(world, phase)
	if !ok {
		return nil
	}
	start := index * EntityCount * EntitySize
	return r.Entities[start : start+EntityCount*EntitySize]
}

// WorldRemap returns the original indirect tile-property table for one phase.
func (r *Resources) WorldRemap(world, phase int) []byte {
	index, ok := worldVariant(world, phase)
	if !ok {
		return nil
	}
	start := index * RemapSize
	return r.Remaps[start : start+RemapSize]
}

// WorldCompositeTiles returns the phase-specific indirect graphic bank.
func (r *Resources) WorldCompositeTiles(world, phase int) []byte {
	index, ok := worldVariant(world, phase)
	if !ok {
		return nil
	}
	start := index * CompositeTileSize
	return r.CompositeTiles[start : start+CompositeTileSize]
}

// WorldRuntimePalette returns the scripted 16-color definition for one phase.
func (r *Resources) WorldRuntimePalette(world, phase int) []byte {
	index, ok := worldVariant(world, phase)
	if !ok {
		return nil
	}
	start := index * RuntimePaletteSize
	return r.RuntimePalettes[start : start+RuntimePaletteSize]
}

// WorldScriptState returns the post-startup variables for one world phase.
func (r *Resources) WorldScriptState(world, phase int) []byte {
	index, ok := worldVariant(world, phase)
	if !ok {
		return nil
	}
	start := index * ScriptStateSize
	return r.ScriptStates[start : start+ScriptStateSize]
}

// WorldScriptPreState returns variables immediately before the startup script.
func (r *Resources) WorldScriptPreState(world, phase int) []byte {
	index, ok := worldVariant(world, phase)
	if !ok {
		return nil
	}
	start := index * ScriptStateSize
	return r.ScriptPreStates[start : start+ScriptStateSize]
}

// WorldScripts returns the main script and its signed-offset table addresses.
func (r *Resources) WorldScripts(world int) (script, offsets int, err error) {
	const base = 0xaa82
	if world < 0 || world >= WorldCount {
		return 0, 0, fmt.Errorf("world script %d is outside recovered data", world)
	}
	pointer := int(binary.BigEndian.Uint32(r.WorldTables[world*4:]))
	descriptor := pointer - base
	if descriptor < 0 || descriptor+0x24 > len(r.WorldTables) {
		return 0, 0, fmt.Errorf("world script descriptor %d is outside recovered data", world)
	}
	return int(binary.BigEndian.Uint32(r.WorldTables[descriptor+0x1c:])), int(binary.BigEndian.Uint32(r.WorldTables[descriptor+0x20:])), nil
}

// WorldLogicAt resolves an original data address without exposing machine code.
func (r *Resources) WorldLogicAt(address int) []byte {
	const worldTableBase = 0xaa82
	if offset := address - worldTableBase; offset >= 0 && offset < len(r.WorldTables) {
		return r.WorldTables[offset:]
	}
	offset := address - WorldLogicBase
	if offset < 0 || offset >= len(r.WorldLogic) {
		return nil
	}
	return r.WorldLogic[offset:]
}

// ResolveCell returns the effective collision code and replacement tile.
// The original map uses the high flag bit to select a phase-specific entry.
func (r *Resources) ResolveCell(world, phase int, flags, tile byte) (collision, replacement byte) {
	if flags&0x80 == 0 {
		return tile, flags
	}
	remap := r.WorldRemap(world, phase)
	offset := int(tile) * 2
	if offset+2 > len(remap) {
		return 0, 0
	}
	word := binary.BigEndian.Uint16(remap[offset:])
	return byte(word), byte(word >> 8)
}

// WorldRuntimeAux returns the original portal, exit and spawn tables.
func (r *Resources) WorldRuntimeAux(world, phase int) []byte {
	index, ok := worldVariant(world, phase)
	if !ok {
		return nil
	}
	start := index * WorldAuxSize
	return r.WorldAux[start : start+WorldAuxSize]
}

// WorldPortal returns one destination from the original phase table.
func (r *Resources) WorldPortal(world, phase, portal int) (Portal, error) {
	descriptor, err := r.World(world, phase)
	if err != nil {
		return Portal{}, err
	}
	if portal < 0 || portal >= descriptor.PortalCount {
		return Portal{}, fmt.Errorf("portal %d is outside world %d phase %d", portal, world, phase)
	}
	data := r.WorldRuntimeAux(world, phase)
	offset := portalOffset + portal*4
	return Portal{int(binary.BigEndian.Uint16(data[offset:])), int(binary.BigEndian.Uint16(data[offset+2:]))}, nil
}

// WorldExit returns one destination from the original phase table.
func (r *Resources) WorldExit(world, phase, exit int) (Exit, error) {
	descriptor, err := r.World(world, phase)
	if err != nil {
		return Exit{}, err
	}
	if exit < 0 || exit >= descriptor.ExitCount {
		return Exit{}, fmt.Errorf("exit %d is outside world %d phase %d", exit, world, phase)
	}
	data := r.WorldRuntimeAux(world, phase)
	offset := exitOffset + exit*10
	return Exit{
		World: int(binary.BigEndian.Uint16(data[offset:])),
		OneX:  int(binary.BigEndian.Uint16(data[offset+2:])), OneY: int(binary.BigEndian.Uint16(data[offset+4:])),
		TwoX: int(binary.BigEndian.Uint16(data[offset+6:])), TwoY: int(binary.BigEndian.Uint16(data[offset+8:])),
	}, nil
}

// WorldSpawn returns one phase-adjusted dynamic actor spawn position.
func (r *Resources) WorldSpawn(world, phase, spawn int) (Portal, error) {
	descriptor, err := r.World(world, phase)
	if err != nil {
		return Portal{}, err
	}
	if spawn < 0 || spawn >= descriptor.SpawnCount {
		return Portal{}, fmt.Errorf("spawn %d is outside world %d phase %d", spawn, world, phase)
	}
	data := r.WorldRuntimeAux(world, phase)
	offset := spawnOffset + spawn*4
	return Portal{int(binary.BigEndian.Uint16(data[offset:])), int(binary.BigEndian.Uint16(data[offset+2:]))}, nil
}

// WorldSpawnKinds returns the original eight-entry actor selection row.
func (r *Resources) WorldSpawnKinds(world, phase int) ([]byte, error) {
	const base = 0xaa82
	if world < 0 || world >= WorldCount {
		return nil, fmt.Errorf("spawn kinds for world %d are outside recovered data", world)
	}
	pointer := int(binary.BigEndian.Uint32(r.WorldTables[world*4:]))
	offset := pointer - base + 0x5c + (phase&0xfc)*2
	if offset < 0 || offset+8 > len(r.WorldTables) {
		return nil, fmt.Errorf("spawn kinds for world %d phase %d are outside recovered data", world, phase)
	}
	return r.WorldTables[offset : offset+8], nil
}

// EntityProperty returns the original descriptor for an actor kind.
func (r *Resources) EntityProperty(kind int) (EntityProperty, error) {
	const size = 16
	offset := kind * size
	if kind < 0 || offset+size > len(r.TileProperties) {
		return EntityProperty{}, fmt.Errorf("entity kind %d is outside recovered properties", kind)
	}
	data := r.TileProperties[offset : offset+size]
	return EntityProperty{
		Graphic: binary.BigEndian.Uint16(data), Value: binary.BigEndian.Uint16(data[2:]),
		Sequence: binary.BigEndian.Uint16(data[4:]), Speed: binary.BigEndian.Uint16(data[6:]),
		Flags: binary.BigEndian.Uint16(data[8:]), Movement: binary.BigEndian.Uint32(data[10:]),
		Vitality: binary.BigEndian.Uint16(data[14:]),
	}, nil
}

// EntityAnimation returns one directional frame and whether the sequence ended.
func (r *Resources) EntityAnimation(sequence, direction, frame int) (int, bool) {
	offset := sequence*32 + direction*8 + frame
	if offset < 0 || offset >= len(r.EntityAnimations) {
		return 0, true
	}
	value := int(int8(r.EntityAnimations[offset]))
	if value < 0 {
		return 0, true
	}
	return value, false
}

// RatingProfile returns one rank name and its original score parameters.
func (r *Resources) RatingProfile(index int) (RatingProfile, error) {
	const profilesOffset = 0x70
	if index < 0 || index >= RatingCount {
		return RatingProfile{}, fmt.Errorf("rating profile %d is outside recovered data", index)
	}
	data := r.RatingData[profilesOffset+index*16 : profilesOffset+(index+1)*16]
	return RatingProfile{
		Name:      strings.TrimRight(string(data[:10]), "\x00 "),
		Threshold: int(binary.BigEndian.Uint16(data[10:])),
		KillBonus: int(binary.BigEndian.Uint16(data[12:])),
		Drift:     int(int16(binary.BigEndian.Uint16(data[14:]))),
	}, nil
}

// RatingForScore resolves the original rank threshold table.
func (r *Resources) RatingForScore(score int) int {
	for index := range RatingCount {
		profile, _ := r.RatingProfile(index)
		if score >= profile.Threshold {
			return index
		}
	}
	return RatingCount - 1
}

// RatingPerformanceCategory resolves the movement-density adjustment band.
func (r *Resources) RatingPerformanceCategory(metric int) int {
	for index := range RatingCount {
		threshold := int(binary.BigEndian.Uint16(r.RatingData[index*2:]))
		if metric >= threshold {
			if index == RatingCount-1 {
				return 4
			}
			return index
		}
	}
	return 4
}

// RatingAgeDivisor returns the weight for one trail-history age.
func (r *Resources) RatingAgeDivisor(age int) int {
	if age < 0 || age >= 32 {
		return 1
	}
	return int(r.RatingData[0x14+age])
}

// RatingDistanceWeight returns the weight for a nearby trail position.
func (r *Resources) RatingDistanceWeight(distance int) int {
	if distance < 0 || distance >= 12 {
		return 0
	}
	return int(binary.BigEndian.Uint16(r.RatingData[0x34+distance*2:]))
}

func worldVariant(world, phase int) (int, bool) {
	if world < 0 || world >= WorldCount || phase < 0 || phase >= PhaseCount {
		return 0, false
	}
	return world*PhaseCount + phase, true
}

func cell(data []byte, x, y int) (flags, tile byte) {
	if x < 0 || x >= 64 || y < 0 || y >= 64 {
		return 0xff, 0
	}
	offset := y*MapStride + x*2
	return data[offset], data[offset+1]
}

// WorldNames reads the 17 original descriptor names in on-disk order.
func (r *Resources) WorldNames() ([]string, error) {
	const base = 0xaa82
	names := make([]string, 0, 17)
	for index := range WorldCount {
		pointer := int(binary.BigEndian.Uint32(r.WorldTables[index*4:]))
		offset := pointer - base + 0x7c
		if offset < 0 || offset >= len(r.WorldTables) {
			return nil, fmt.Errorf("world descriptor %d is outside recovered data", index)
		}
		end := offset
		for end < len(r.WorldTables) && r.WorldTables[end] != 0 {
			end++
		}
		if end == len(r.WorldTables) {
			return nil, fmt.Errorf("unterminated world descriptor %d", index)
		}
		names = append(names, string(r.WorldTables[offset:end]))
	}
	return names, nil
}

// WorldPalette returns the original palette selector stored in a descriptor.
func (r *Resources) WorldPalette(world int) (int, error) {
	const base = 0xaa82
	if world < 0 || world >= WorldCount {
		return 0, fmt.Errorf("world index %d is outside the descriptor table", world)
	}
	pointer := int(binary.BigEndian.Uint32(r.WorldTables[world*4:]))
	offset := pointer - base + 0x5b
	if offset < 0 || offset >= len(r.WorldTables) {
		return 0, fmt.Errorf("world palette %d is outside recovered data", world)
	}
	return int(r.WorldTables[offset]), nil
}
