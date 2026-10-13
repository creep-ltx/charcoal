package main

import "testing"

// run6502 runs the few instructions writePrg uses until the program calls
// GETIN ($FFE4), and returns the memory.
func run6502(t *testing.T, prg []byte) *[65536]byte {
	var m [65536]byte
	load := int(prg[0]) | int(prg[1])<<8
	copy(m[load:], prg[2:])
	// The BASIC line must say SYS 2061.
	if string(m[0x0806:0x080A]) != "2061" || m[0x0805] != 0x9E {
		t.Fatalf("BASIC line: % x", m[0x0801:0x080D])
	}
	pc, x, a := 2061, 0, 0
	var z bool
	abs := func() int { return int(m[pc+1]) | int(m[pc+2])<<8 }
	for steps := 0; steps < 100000; steps++ {
		switch op := m[pc]; op {
		case 0xA9:
			a, z = int(m[pc+1]), m[pc+1] == 0
			pc += 2
		case 0xA2:
			x, z = int(m[pc+1]), m[pc+1] == 0
			pc += 2
		case 0x8D:
			m[abs()] = byte(a)
			pc += 3
		case 0xBD:
			a = int(m[(abs()+x)&0xFFFF])
			z = a == 0
			pc += 3
		case 0x9D:
			m[(abs()+x)&0xFFFF] = byte(a)
			pc += 3
		case 0xE8:
			x = (x + 1) & 255
			z = x == 0
			pc++
		case 0xE0:
			z = x == int(m[pc+1])
			pc += 2
		case 0xD0, 0xF0:
			taken := (op == 0xD0) != z
			pc += 2
			if taken {
				pc += int(int8(m[pc-1]))
			}
		case 0x20:
			if abs() == 0xFFE4 {
				return &m
			}
			t.Fatalf("JSR %04X before waiting for a key", abs())
		default:
			t.Fatalf("unexpected opcode %02X at %04X", op, pc)
		}
	}
	t.Fatal("the program never waits for a key")
	return nil
}

func TestPrg(t *testing.T) {
	d := newDoc(40, 25)
	d.bg, d.border = 6, 14
	for i := 0; i < 1000; i++ {
		d.cells[i] = makeCell(i%256, i%16, 0, 0)
	}
	m := run6502(t, writePrg(d, true))
	for i := 0; i < 1000; i++ {
		if int(m[0x0400+i]) != i%256 || int(m[0xD800+i]) != i%16 {
			t.Fatalf("cell %d: screen %d colour %d", i, m[0x0400+i], m[0xD800+i])
		}
	}
	if m[0xD020] != 14 || m[0xD021] != 6 || m[0xD018] != 0x17 {
		t.Fatalf("border %d bg %d charset %02X", m[0xD020], m[0xD021], m[0xD018])
	}
}

func TestMarq(t *testing.T) {
	d := newDoc(40, 25)
	d.bg, d.border = 0, 11
	d.set(3, 2, makeCell(0xA0, 7, 0, 0))
	back, lower, err := parseMarq(writeMarq(d, false))
	if err != nil || lower != 0 || back.get(3, 2) != d.get(3, 2) || back.border != 11 {
		t.Fatalf("marq: %v %d %x %d", err, lower, back.get(3, 2), back.border)
	}
}

func TestXBin(t *testing.T) {
	d := newDoc(80, 3)
	d.set(5, 1, makeCell(0xDB, 12, 1, 0))
	pal := palettes[1].colors
	back, info, err := parseXBin(writeXBin(d, pal, fontByID("vga"), 16, true))
	if err != nil || back.get(5, 1) != d.get(5, 1) || !info.ice || info.pal[4] != pal[4] || info.font.bits[0xDB*16+5] != fontByID("vga").bits[0xDB*16+5] {
		t.Fatalf("xbin: %v %x", err, back.get(5, 1))
	}
	// A compressed one, by hand: 4 columns, 1 row, no font or palette.
	// Run 1 (both): 'A' attr 7 twice. Run 2 (char): 'B' with attrs 1, 2.
	b := []byte{'X', 'B', 'I', 'N', 0x1A, 4, 0, 1, 0, 16, 4, 0xC1, 'A', 7, 0x41, 'B', 1, 2}
	c, _, err := parseXBin(b)
	if err != nil || c.get(1, 0) != makeCell('A', 7, 0, 0) || c.get(3, 0) != makeCell('B', 2, 0, 0) {
		t.Fatalf("compressed: %v %x %x", err, c.get(1, 0), c.get(3, 0))
	}
}
