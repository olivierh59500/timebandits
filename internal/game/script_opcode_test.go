package game

import (
	"encoding/binary"
	"testing"

	"github.com/olivierh59500/timebandits/internal/original"
)

func TestAllReachableWorldScriptOpcodesAreSupported(t *testing.T) {
	resources, err := original.Load()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[byte]bool{}
	for world := 0; world < original.WorldCount; world++ {
		base, offsets, err := resources.WorldScripts(world)
		if err != nil {
			t.Fatal(err)
		}
		entries := map[int]bool{base: true}
		for phase := 0; phase < original.PhaseCount; phase++ {
			descriptor, err := resources.World(world, phase)
			if err != nil {
				t.Fatal(err)
			}
			for y := 0; y < descriptor.Height; y++ {
				for x := 0; x < descriptor.Width; x++ {
					flags, _ := resources.WorldCell(world, phase, x, y)
					if flags>>5&3 != 1 {
						continue
					}
					data := resources.WorldLogicAt(offsets + int(flags&0x1f)*2)
					if len(data) >= 2 {
						entries[base+int(int16(binary.BigEndian.Uint16(data)))] = true
					}
				}
			}
		}
		visited := map[int]bool{}
		var walk func(int)
		walk = func(pc int) {
			for operations := 0; operations < 10000 && !visited[pc]; operations++ {
				visited[pc] = true
				data := resources.WorldLogicAt(pc)
				if len(data) == 0 {
					return
				}
				opcode := data[0]
				seen[opcode] = true
				pc++
				if int(opcode) >= len(scriptOperandLengths) {
					t.Fatalf("world %d has unsupported opcode %02x at %x", world, opcode, pc-1)
				}
				switch opcode {
				case 0x03:
					for {
						value := resources.WorldLogicAt(pc)[0]
						pc++
						if value == 0 {
							break
						}
						if value == 2 || value == 7 {
							pc++
						} else if value == 3 {
							pc += 2
						}
					}
				case 0x0c:
					pc++
					for resources.WorldLogicAt(pc)[0] != 0 {
						pc++
					}
					pc++
				case 0x06, 0x07:
					operands := resources.WorldLogicAt(pc)
					if len(operands) < 2 {
						return
					}
					pc += 2
					if opcode == 0x06 && operands[0] == 0 {
						return
					}
				case 0x15, 0x16:
					index := int(resources.WorldLogicAt(pc)[0])
					targetData := resources.WorldLogicAt(offsets + index*2)
					if len(targetData) >= 2 {
						walk(base + int(int16(binary.BigEndian.Uint16(targetData))))
					}
					pc++
					if opcode == 0x15 {
						return
					}
				case 0x0b, 0x17, 0x1a, 0x1b, 0x29, 0x2e:
					return
				default:
					pc += scriptOperandLengths[opcode]
				}
			}
		}
		for entry := range entries {
			walk(entry)
		}
	}
	if len(seen) < 30 {
		t.Fatalf("script coverage reached only %d distinct opcodes", len(seen))
	}
}
