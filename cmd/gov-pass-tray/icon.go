package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
)

type rgba struct{ r, g, b, a uint8 }

func mustBuildIcoCircle(size int, c rgba) []byte {
	ico, err := buildIcoCircle(size, c)
	if err != nil {
		return mustBuildEmptyIco()
	}
	return ico
}

func mustBuildEmptyIco() []byte {
	ico, err := buildIcoTransparent(16)
	if err != nil {
		// 1x1 transparent PNG inside a minimal ICO container.
		png1 := []byte{
			0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
			0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
			0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
			0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
			0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
			0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
			0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
			0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
			0x42, 0x60, 0x82,
		}
		ico, _ := buildIcoFromPNGs([]pngEntry{{w: 1, h: 1, png: png1}})
		return ico
	}
	return ico
}

func buildIcoCircle(size int, c rgba) ([]byte, error) {
	if size <= 0 {
		return nil, errors.New("size must be > 0")
	}

	sizes := []int{16, size}
	seen := make(map[int]bool, len(sizes))
	var uniq []int
	for _, s := range sizes {
		if s <= 0 {
			continue
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		uniq = append(uniq, s)
	}
	if len(uniq) == 0 {
		uniq = []int{16}
	}
	sort.Ints(uniq)

	entries := make([]pngEntry, 0, len(uniq))
	for _, s := range uniq {
		pb, err := buildPNGCircle(s, c)
		if err != nil {
			return nil, err
		}
		entries = append(entries, pngEntry{w: s, h: s, png: pb})
	}

	return buildIcoFromPNGs(entries)
}

func buildIcoTransparent(size int) ([]byte, error) {
	pb, err := buildPNGTransparent(size)
	if err != nil {
		return nil, err
	}
	return buildIcoFromPNGs([]pngEntry{{w: size, h: size, png: pb}})
}

type pngEntry struct {
	w, h int
	png  []byte
}

func buildIcoFromPNGs(entries []pngEntry) ([]byte, error) {
	if len(entries) < 1 {
		return nil, errors.New("no png entries")
	}

	buf := new(bytes.Buffer)

	// ICONDIR
	if err := binary.Write(buf, binary.LittleEndian, uint16(0)); err != nil { // reserved
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(1)); err != nil { // type = icon
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, uint16(len(entries))); err != nil {
		return nil, err
	}

	// ICONDIRENTRY array.
	offset := 6 + 16*len(entries)
	for _, e := range entries {
		if e.w <= 0 || e.h <= 0 {
			return nil, errors.New("invalid png dimensions")
		}
		if len(e.png) == 0 {
			return nil, errors.New("empty png")
		}

		w := uint8(e.w)
		h := uint8(e.h)
		if e.w >= 256 {
			w = 0
		}
		if e.h >= 256 {
			h = 0
		}

		if err := buf.WriteByte(w); err != nil { // width
			return nil, err
		}
		if err := buf.WriteByte(h); err != nil { // height
			return nil, err
		}
		if err := buf.WriteByte(0); err != nil { // colors
			return nil, err
		}
		if err := buf.WriteByte(0); err != nil { // reserved
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, uint16(1)); err != nil { // planes
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, uint16(32)); err != nil { // bit count
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, uint32(len(e.png))); err != nil {
			return nil, err
		}
		if err := binary.Write(buf, binary.LittleEndian, uint32(offset)); err != nil {
			return nil, err
		}

		offset += len(e.png)
	}

	for _, e := range entries {
		if _, err := buf.Write(e.png); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func buildPNGCircle(size int, c rgba) ([]byte, error) {
	if size <= 0 {
		return nil, errors.New("size must be > 0")
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))

	// Apple-style squircle (superellipse) shape.
	cx := float64(size) / 2
	cy := float64(size) / 2
	pad := float64(size) * 0.08
	rx := (float64(size) - 2*pad) / 2
	ry := rx
	n := 4.5 // continuous-curvature exponent

	const samples = 4 // 4×4 super-sampling for anti-aliased edges
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var coverage float64
			for sy := 0; sy < samples; sy++ {
				for sx := 0; sx < samples; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(samples)
					py := float64(y) + (float64(sy)+0.5)/float64(samples)
					dx := math.Abs(px-cx) / rx
					dy := math.Abs(py-cy) / ry
					if math.Pow(dx, n)+math.Pow(dy, n) <= 1.0 {
						coverage += 1.0
					}
				}
			}
			coverage /= float64(samples * samples)

			if coverage > 0 {
				// Subtle vertical gradient for depth (brighter top, darker bottom).
				t := float64(y) / float64(size)
				brighten := 1.0 + 0.10*(0.5-t)

				r := clampByte(float64(c.r) * brighten)
				g := clampByte(float64(c.g) * brighten)
				b := clampByte(float64(c.b) * brighten)
				a := uint8(math.Round(float64(c.a) * coverage))

				img.SetNRGBA(x, y, color.NRGBA{R: r, G: g, B: b, A: a})
			}
		}
	}

	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(math.Round(v))
}

func buildPNGTransparent(size int) ([]byte, error) {
	if size <= 0 {
		return nil, errors.New("size must be > 0")
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
