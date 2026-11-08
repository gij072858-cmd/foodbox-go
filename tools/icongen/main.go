//go:build ignore

// icongen renders the app icon: the two characters 食匣 side by side
// horizontally in a bold sans (黑体) on white, at 1024×1024. The executable
// and installer icon is a static asset, so it cannot follow the runtime
// theme; white is the agreed fallback.
//
//	go run ./tools/icongen -out resources/icon.png
//
// It is a generator, not part of the app; the build tag keeps it out of
// go build ./....
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var fontCandidates = []string{
	`C:\Windows\Fonts\msyhbd.ttc`, // Microsoft YaHei Bold
	`C:\Windows\Fonts\simhei.ttf`, // SimHei
	`C:\Windows\Fonts\NotoSansSC-VF.ttf`,
}

func main() {
	out := flag.String("out", "resources/icon.png", "output PNG")
	fontPath := flag.String("font", "", "font file (default: try Windows bold sans fonts)")
	size := flag.Int("size", 1024, "final icon size")
	flag.Parse()

	path := *fontPath
	if path == "" {
		for _, candidate := range fontCandidates {
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
				break
			}
		}
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "icongen: no font found; pass -font")
		os.Exit(1)
	}
	face, err := loadFace(path, float64(*size*2)*0.36)
	if err != nil {
		fmt.Fprintln(os.Stderr, "icongen:", err)
		os.Exit(1)
	}
	defer face.Close()

	big := image.NewRGBA(image.Rect(0, 0, *size*2, *size*2))
	draw.Draw(big, big.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	drawHorizontal(big, face, "食匣")
	final := image.NewRGBA(image.Rect(0, 0, *size, *size))
	draw.CatmullRom.Scale(final, final.Bounds(), big, big.Bounds(), draw.Src, nil)

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "icongen:", err)
		os.Exit(1)
	}
	if err := png.Encode(f, final); err != nil {
		_ = f.Close()
		fmt.Fprintln(os.Stderr, "icongen:", err)
		os.Exit(1)
	}
	if err := f.Close(); err != nil {
		fmt.Fprintln(os.Stderr, "icongen:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s from %s\n", *out, path)
}

func loadFace(path string, size float64) (font.Face, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f *opentype.Font
	if len(path) > 4 && path[len(path)-4:] == ".ttc" {
		col, err := opentype.ParseCollection(data)
		if err != nil {
			return nil, err
		}
		f, err = col.Font(0)
		if err != nil {
			return nil, err
		}
	} else {
		f, err = opentype.Parse(data)
		if err != nil {
			return nil, err
		}
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size: size, DPI: 72, Hinting: font.HintingFull,
	})
}

// drawHorizontal centres 食匣 side by side, using the glyphs' ink box so the
// pair reads as one mark on the square rather than by advance width alone.
func drawHorizontal(dst *image.RGBA, face font.Face, text string) {
	ink := color.RGBA{R: 0x1C, G: 0x19, B: 0x17, A: 0xFF}
	b, _ := font.BoundString(face, text)
	w := float64(b.Max.X-b.Min.X) / 64
	h := float64(b.Max.Y-b.Min.Y) / 64
	x := (float64(dst.Bounds().Dx())-w)/2 - float64(b.Min.X)/64
	top := (float64(dst.Bounds().Dy()) - h) / 2
	baseline := top - float64(b.Min.Y)/64
	d := font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(ink),
		Face: face,
		Dot:  fixed.P(int(math.Round(x)), int(math.Round(baseline))),
	}
	d.DrawString(text)
}
