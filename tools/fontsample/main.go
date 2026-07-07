package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"

	"github.com/go-restruct/restruct"

	"lucksystem/font"
)

func main() {
	restruct.EnableExprBeta()
	if len(os.Args) != 5 {
		fmt.Fprintln(os.Stderr, "usage: fontsample <info-file> <glyph-file> <text> <output-png>")
		os.Exit(1)
	}
	lucaFont := font.LoadLucaFontFile(os.Args[1], os.Args[2])
	img := lucaFont.GetStringImage(os.Args[3])
	canvas := image.NewNRGBA(img.Bounds())
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.NRGBA{R: 32, G: 32, B: 36, A: 255}}, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), img, img.Bounds().Min, draw.Over)
	out, err := os.Create(os.Args[4])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer out.Close()
	if err := png.Encode(out, canvas); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
