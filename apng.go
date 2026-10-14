package main

// APNG by hand (no library): every frame is a PNG of the same palette, and
// its IDAT data goes into the animation as the first frame's IDAT or a later
// frame's fdAT, after an fcTL saying where it goes and how long it stays.
// (APNG: https://wiki.mozilla.org/APNG_Specification.) A viewer without APNG
// shows the first frame as a plain PNG.

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image/png"
)

// pngChunks splits a PNG into its chunks (type and data).
func pngChunks(b []byte) (types []string, datas [][]byte, err error) {
	if len(b) < 8 || string(b[:8]) != "\x89PNG\r\n\x1a\n" {
		return nil, nil, fmt.Errorf("not a PNG")
	}
	for i := 8; i < len(b); {
		if i+12 > len(b) {
			return nil, nil, fmt.Errorf("a chunk is cut short")
		}
		n := int(binary.BigEndian.Uint32(b[i:]))
		if n < 0 || i+12+n > len(b) {
			return nil, nil, fmt.Errorf("a chunk is cut short")
		}
		types = append(types, string(b[i+4:i+8]))
		datas = append(datas, b[i+8:i+8+n])
		i += 12 + n
	}
	return types, datas, nil
}

// pngChunk writes one chunk with its length and CRC.
func pngChunk(out *bytes.Buffer, typ string, data []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(data)))
	out.Write(n[:])
	crc := crc32.NewIEEE()
	crc.Write([]byte(typ))
	crc.Write(data)
	out.WriteString(typ)
	out.Write(data)
	binary.BigEndian.PutUint32(n[:], crc.Sum32())
	out.Write(n[:])
}

func be32(v ...int) []byte {
	b := make([]byte, 0, 4*len(v))
	for _, x := range v {
		b = binary.BigEndian.AppendUint32(b, uint32(x))
	}
	return b
}

// writeAPNG writes the frames as an APNG: signature, the first frame's IHDR
// and PLTE (one palette for all), acTL (frames, plays: 0 for ever), then per
// frame fcTL (sequence, size, place, delay in ms/1000, dispose none, blend
// source) and its image data, IDAT for the first and fdAT (with the next
// sequence number) for the others; one sequence runs over fcTL and fdAT.
func writeAPNG(frames []animFrame, w, h int, loop bool) ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("\x89PNG\r\n\x1a\n")
	seq := 0
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	for i, f := range frames {
		var one bytes.Buffer
		if err := enc.Encode(&one, f.img); err != nil {
			return nil, err
		}
		types, datas, err := pngChunks(one.Bytes())
		if err != nil {
			return nil, err
		}
		if i == 0 {
			for j, t := range types {
				if t == "IHDR" || t == "PLTE" || t == "tRNS" {
					pngChunk(&out, t, datas[j])
				}
			}
			plays := 0
			if !loop {
				plays = 1
			}
			pngChunk(&out, "acTL", be32(len(frames), plays))
		}
		r := f.img.Rect
		fc := be32(seq, r.Dx(), r.Dy(), r.Min.X, r.Min.Y)
		fc = append(fc, byte(min(f.delay, 65535)>>8), byte(min(f.delay, 65535)), 1000>>8, 1000&255, 0, 0)
		pngChunk(&out, "fcTL", fc)
		seq++
		for j, t := range types {
			if t != "IDAT" {
				continue
			}
			if i == 0 {
				pngChunk(&out, "IDAT", datas[j])
			} else {
				pngChunk(&out, "fdAT", append(be32(seq), datas[j]...))
				seq++
			}
		}
	}
	pngChunk(&out, "IEND", nil)
	return out.Bytes(), nil
}
