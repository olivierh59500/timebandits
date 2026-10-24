package original

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/color"
	"reflect"
	"testing"
)

func TestRecoveredResources(t *testing.T) {
	resources, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(resources.Tiles), TileCount*TileSize; got != want {
		t.Fatalf("tile bank length = %d, want %d", got, want)
	}
	if flags, tile := resources.Cell(5, 5); flags != 0 || tile != 0xbf {
		t.Fatalf("initial player cell = %02x/%02x, want 00/bf", flags, tile)
	}
	palette := resources.Palette(4, 0)
	want := []color.Color{
		color.NRGBA{0, 0, 0, 255},
		color.NRGBA{0, 136, 0, 255},
		color.NRGBA{0, 238, 238, 255},
	}
	if !reflect.DeepEqual([]color.Color(palette[:3]), want) {
		t.Fatalf("first palette colors = %v, want %v", palette[:3], want)
	}
	atlas, err := DecodeTiles(resources.Tiles, palette)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := atlas.Bounds().Dx(), 256; got != want {
		t.Fatalf("atlas width = %d, want %d", got, want)
	}
	timeLords, err := DecodePlanarGraphic(resources.TimeLordsLogo, TimeLordsLogoWidth, TimeLordsLogoHeight, resources.Palette(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	todaysHeroes, err := DecodePlanarGraphic(resources.TodaysHeroesLogo, TodaysHeroesLogoWidth, TodaysHeroesLogoHeight, resources.Palette(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if timeLords.Bounds() != image.Rect(0, 0, 48, 71) || todaysHeroes.Bounds() != image.Rect(0, 0, 80, 71) {
		t.Fatalf("Timelord logos have unexpected bounds: %v / %v", timeLords.Bounds(), todaysHeroes.Bounds())
	}
	titleLogo, err := DecodePlanarGraphic(resources.TitleLogo, TitleLogoWidth, TitleLogoHeight, resources.Palette(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if titleLogo.Bounds() != image.Rect(0, 0, 288, 84) {
		t.Fatalf("title logo bounds = %v", titleLogo.Bounds())
	}
	hudLogo, err := DecodePlanarGraphic(resources.HUDLogo, HUDLogoWidth, HUDLogoHeight, resources.Palette(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if hudLogo.Bounds() != image.Rect(0, 0, 112, 113) {
		t.Fatalf("HUD logo bounds = %v", hudLogo.Bounds())
	}
	masks, err := DecodeMasks(resources.SpawnMasks)
	if err != nil {
		t.Fatal(err)
	}
	opaque := 0
	for y := range 16 {
		for x := range 16 {
			if masks.AlphaAt(7*16+x, y).A != 0 {
				opaque++
			}
		}
	}
	if masks.AlphaAt(0, 0).A != 255 || masks.AlphaAt(16, 0).A != 0 || opaque != 172 {
		t.Fatalf("materialization masks = full %d, empty %d, final pixels %d", masks.AlphaAt(0, 0).A, masks.AlphaAt(16, 0).A, opaque)
	}
	banditMasks, err := DecodeInverseMasks(resources.BanditOne)
	if err != nil {
		t.Fatal(err)
	}
	opaque = 0
	for y := range 16 {
		for x := range 16 {
			if banditMasks.AlphaAt(x, y).A != 0 {
				opaque++
			}
		}
	}
	if banditMasks.Bounds() != image.Rect(0, 0, BanditCount*16, 16) || banditMasks.AlphaAt(0, 0).A != 0 || banditMasks.AlphaAt(5, 0).A != 255 || opaque != 127 {
		t.Fatalf("Bandit mask bounds/pixels = %v/%d/%d/%d", banditMasks.Bounds(), banditMasks.AlphaAt(0, 0).A, banditMasks.AlphaAt(5, 0).A, opaque)
	}
}

func TestRecoveredWorldDataMatchesDescriptors(t *testing.T) {
	resources, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	seenMaps := make([]map[string]bool, WorldCount)
	for world := range seenMaps {
		seenMaps[world] = make(map[string]bool)
	}
	for index, world := range resources.Worlds {
		mapData := resources.Maps[index*MapCapacity : (index+1)*MapCapacity]
		mapDigest := fmt.Sprintf("%x", sha256.Sum256(mapData))
		if mapDigest != world.MapSHA256 {
			t.Fatalf("world %d map digest = %s, want %s", index, mapDigest, world.MapSHA256)
		}
		seenMaps[world.World][mapDigest] = true
		entities := resources.WorldEntities(world.World, world.Phase)
		entityDigest := fmt.Sprintf("%x", sha256.Sum256(entities))
		if entityDigest != world.EntitiesSHA256 {
			t.Fatalf("world %d phase %d entity digest = %s, want %s", world.World, world.Phase, entityDigest, world.EntitiesSHA256)
		}
		if len(entities) != EntityCount*EntitySize {
			t.Fatalf("world %d entity data length = %d", index, len(entities))
		}
		remap := resources.WorldRemap(world.World, world.Phase)
		if digest := fmt.Sprintf("%x", sha256.Sum256(remap)); digest != world.RemapSHA256 {
			t.Fatalf("world %d phase %d remap digest = %s, want %s", world.World, world.Phase, digest, world.RemapSHA256)
		}
		aux := resources.WorldRuntimeAux(world.World, world.Phase)
		if digest := fmt.Sprintf("%x", sha256.Sum256(aux)); digest != world.AuxSHA256 {
			t.Fatalf("world %d phase %d auxiliary digest = %s, want %s", world.World, world.Phase, digest, world.AuxSHA256)
		}
		composites := resources.WorldCompositeTiles(world.World, world.Phase)
		if digest := fmt.Sprintf("%x", sha256.Sum256(composites)); digest != world.CompositeSHA256 {
			t.Fatalf("world %d phase %d composite digest = %s, want %s", world.World, world.Phase, digest, world.CompositeSHA256)
		}
		palette := resources.WorldRuntimePalette(world.World, world.Phase)
		if digest := fmt.Sprintf("%x", sha256.Sum256(palette)); digest != world.PaletteSHA256 {
			t.Fatalf("world %d phase %d palette digest = %s, want %s", world.World, world.Phase, digest, world.PaletteSHA256)
		}
		state := resources.WorldScriptState(world.World, world.Phase)
		if digest := fmt.Sprintf("%x", sha256.Sum256(state)); digest != world.ScriptStateSHA256 {
			t.Fatalf("world %d phase %d script-state digest = %s, want %s", world.World, world.Phase, digest, world.ScriptStateSHA256)
		}
		preState := resources.WorldScriptPreState(world.World, world.Phase)
		if digest := fmt.Sprintf("%x", sha256.Sum256(preState)); digest != world.ScriptPreSHA256 {
			t.Fatalf("world %d phase %d pre-script digest = %s, want %s", world.World, world.Phase, digest, world.ScriptPreSHA256)
		}
		if world.Width < 19 || world.Width > 64 || world.Height < 21 || world.Height > 64 {
			t.Fatalf("world %d dimensions = %dx%d", index, world.Width, world.Height)
		}
	}
	if got := len(seenMaps[0]); got != 1 {
		t.Fatalf("Timegates variants = %d, want 1", got)
	}
	if got := len(seenMaps[1]); got != PhaseCount {
		t.Fatalf("Castle Greymoon variants = %d, want %d", got, PhaseCount)
	}
	flags, tile := resources.WorldCell(0, 0, 5, 5)
	if collision, _ := resources.ResolveCell(0, 0, flags, tile); collision != 0xbf {
		t.Fatalf("Timegates start collision = %02x, want bf", collision)
	}
	if palette := resources.WorldColors(6, 0, 0); reflect.DeepEqual(palette[1:], make(color.Palette, 15)) {
		t.Fatal("Omega Complex retained the empty pre-script palette")
	}
	for world, want := range []int{4, 2, 5} {
		got, err := resources.WorldPalette(world)
		if err != nil || got != want {
			t.Fatalf("world %d palette = %d, %v; want %d", world, got, err, want)
		}
	}
}

func TestWorldDescriptorNames(t *testing.T) {
	resources, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	names, err := resources.WorldNames()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"THE TIMEGATES", "CASTLE GREYMOON", "OLD BOMB FACTORY", "KING'S CROWN", "MAJOR HAZARD", "SHADOWLAND", "OMEGA COMPLEX", "THE EXCALIBUR", "HOTEL CALIFORNIA", "DARKSIDE DARE", "CHEOPS' CURSE", "GRIDVILLE", "THE GUARDIAN", "THE SENTINEL", "WELKIN ISLAND", "UNDERWORLD ARENA", "GHOST TOWN"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("world names = %#v", names)
	}
}

func TestRecoveredWorldNavigationTables(t *testing.T) {
	resources, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	portal, err := resources.WorldPortal(1, 0, 0)
	if err != nil || portal != (Portal{2, 39}) {
		t.Fatalf("Castle Greymoon portal = %+v, %v", portal, err)
	}
	exit, err := resources.WorldExit(0, 0, 1)
	if err != nil || exit != (Exit{World: 1, OneX: 7, OneY: 31, TwoX: 19, TwoY: 30}) {
		t.Fatalf("Timegates exit = %+v, %v", exit, err)
	}
	returnExit, err := resources.WorldExit(1, 0, 0)
	if err != nil || returnExit != (Exit{World: 0, OneX: 13, OneY: 14, TwoX: 11, TwoY: 13}) {
		t.Fatalf("Castle Greymoon exit = %+v, %v", returnExit, err)
	}
	spawn, err := resources.WorldSpawn(1, 0, 0)
	if err != nil || spawn != (Portal{16, 28}) {
		t.Fatalf("Castle Greymoon spawn = %+v, %v", spawn, err)
	}
	kinds, err := resources.WorldSpawnKinds(1, 0)
	if err != nil || !reflect.DeepEqual(kinds, []byte{57, 60, 57, 60, 57, 60, 57, 94}) {
		t.Fatalf("Castle Greymoon spawn kinds = %v, %v", kinds, err)
	}
}

func TestRecoveredWorldScriptPointers(t *testing.T) {
	resources, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	script, offsets, err := resources.WorldScripts(6)
	if err != nil || script != 0xcb7e || offsets != 0 {
		t.Fatalf("Omega Complex scripts = %x/%x, %v", script, offsets, err)
	}
	logic := resources.WorldLogicAt(script)
	if len(logic) == 0 || logic[0] != 0x08 {
		t.Fatalf("Omega Complex script begins with %x", logic[:min(len(logic), 4)])
	}
}

func TestRecoveredEntityAnimationTables(t *testing.T) {
	resources, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	property, err := resources.EntityProperty(1)
	if err != nil {
		t.Fatal(err)
	}
	want := EntityProperty{Graphic: 0x3a, Speed: 2, Movement: 0xffffffff}
	if property.Graphic != want.Graphic || property.Speed != want.Speed || property.Movement != want.Movement {
		t.Fatalf("projectile property = %+v", property)
	}
	if frame, ended := resources.EntityAnimation(int(property.Sequence), 1, 0); ended || frame != 2 {
		t.Fatalf("right projectile animation = %d, ended %t", frame, ended)
	}
	if frame, ended := resources.EntityAnimation(4, 0, 6); !ended || frame != 0 {
		t.Fatalf("terminated animation = %d, ended %t", frame, ended)
	}
}

func TestRecoveredAdaptiveRatingTables(t *testing.T) {
	resources, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	average, err := resources.RatingProfile(5)
	if err != nil || average != (RatingProfile{Name: "AVERAGE", Threshold: 334, KillBonus: 2, Drift: -4}) {
		t.Fatalf("average rating = %+v, %v", average, err)
	}
	if got := resources.RatingForScore(450); got != 5 {
		t.Fatalf("rating for score 450 = %d, want AVERAGE", got)
	}
	if got := resources.RatingPerformanceCategory(600); got != 8 {
		t.Fatalf("performance category at 600 = %d, want 8", got)
	}
	if got := resources.RatingPerformanceCategory(0); got != 4 {
		t.Fatalf("performance category at zero = %d, want fallback 4", got)
	}
	if resources.RatingAgeDivisor(31) != 1 || resources.RatingDistanceWeight(0) != 6 {
		t.Fatal("rating trail weights differ from the recovered table")
	}
}

func TestRecoveredSoundEffectsHaveSilentBoundaries(t *testing.T) {
	resources, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for effect, pcm := range resources.Effects {
		if len(pcm) < 8 || !bytes.Equal(pcm[:4], make([]byte, 4)) || !bytes.Equal(pcm[len(pcm)-4:], make([]byte, 4)) {
			t.Fatalf("effect %d does not have click-free PCM boundaries", effect)
		}
	}
}
