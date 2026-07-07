package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"unicode"

	"github.com/go-restruct/restruct"
	"golang.org/x/image/math/fixed"

	"lucksystem/charset"
	"lucksystem/font"
	"lucksystem/pak"
)

type pair struct {
	InfoIndex  int
	GlyphIndex int
}

func main() {
	restruct.EnableExprBeta()

	yOffset := flag.Int("yoffset", 2, "extra signed vertical offset for injected characters")
	normalizeY := flag.Bool("normalizey", true, "normalize injected lowercase/uppercase y offsets to reference glyphs")
	scale := flag.Float64("scale", 1.0, "temporary TTF render scale relative to each Luca font size")
	forceAll := flag.Bool("forceall", false, "inject every requested character instead of only characters missing from the original table")
	flag.Parse()

	if flag.NArg() != 4 {
		fatalf("usage: sysfontpatch [-yoffset N] <SYSFONT.PAK> <charset-file> <ttf-file> <output-pak>")
	}

	src := flag.Arg(0)
	charsetFile := flag.Arg(1)
	ttfFile := flag.Arg(2)
	outName := flag.Arg(3)

	charsBytes, err := os.ReadFile(charsetFile)
	check(err)
	chars := uniqueRunes([]rune(string(bytes.TrimRight(charsBytes, "\r\n"))))
	if len(chars) == 0 {
		fatalf("empty charset: %s", charsetFile)
	}

	ttfBytes, err := os.ReadFile(ttfFile)
	check(err)

	p := pak.LoadPak(src, charset.UTF_8)
	p.ReadAll()

	// SYSFONT.PAK layout:
	// 0..5 are info entries; 6..11 are their matching glyph atlas entries.
	pairs := []pair{
		{0, 6},
		{1, 7},
		{2, 8},
		{3, 9},
		{4, 10},
		{5, 11},
	}

	for _, current := range pairs {
		infoEntry, err := p.GetByIndex(current.InfoIndex)
		check(err)
		glyphEntry, err := p.GetByIndex(current.GlyphIndex)
		check(err)

		lucaFont := font.LoadLucaFont(infoEntry.Data, glyphEntry.Data)
		patchRunes := missingRunes(lucaFont.Info, chars)
		if *forceAll {
			patchRunes = chars
		}
		if len(patchRunes) == 0 {
			fmt.Printf("skip %s/%s: already contains all requested chars\n", infoEntry.Name, glyphEntry.Name)
			continue
		}
		if int(lucaFont.Info.CharNum) < len(patchRunes) {
			fmt.Printf("skip %s/%s: only %d cells for %d chars\n",
				infoEntry.Name, glyphEntry.Name, lucaFont.Info.CharNum, len(patchRunes))
			continue
		}

		startIndex := int(lucaFont.Info.CharNum) - len(patchRunes)
		lowerY := referenceY(lucaFont.Info, []rune{'a', 'o'})
		upperY := referenceY(lucaFont.Info, []rune{'A', 'O'})
		originalY := originalYByRune(lucaFont.Info, patchRunes)
		originalFontSize := lucaFont.Info.FontSize
		if *scale > 0 && *scale != 1 {
			scaled := int(math.Round(float64(originalFontSize) * *scale))
			if scaled < 1 {
				scaled = 1
			}
			lucaFont.Info.FontSize = uint16(scaled)
		}
		lucaFont.ReplaceChars(bytes.NewReader(ttfBytes), string(patchRunes), startIndex, false)
		redrawRunesInCells(lucaFont, patchRunes)
		lucaFont.Info.FontSize = originalFontSize
		forceIndexMappings(lucaFont.Info, patchRunes, startIndex)
		if *normalizeY {
			normalizeVerticalMetricsTo(lucaFont.Info, patchRunes, lowerY, upperY, originalY, *yOffset)
		} else if *yOffset != 0 {
			offsetVerticalMetrics(lucaFont.Info, patchRunes, *yOffset)
		}

		var glyphOut bytes.Buffer
		var infoOut bytes.Buffer
		check(lucaFont.Write(&glyphOut, &infoOut))
		check(p.SetByIndex(current.GlyphIndex, bytes.NewReader(glyphOut.Bytes())))
		check(p.SetByIndex(current.InfoIndex, bytes.NewReader(infoOut.Bytes())))
		fmt.Printf("patched %s/%s cells=%d start=%d\n",
			infoEntry.Name, glyphEntry.Name, lucaFont.Info.CharNum, startIndex)
	}

	p.Rebuild = true
	out, err := os.Create(outName)
	check(err)
	check(p.Write(out))
	check(out.Close())
	fmt.Println(outName)
}

func missingRunes(info *font.Info, chars []rune) []rune {
	seen := make(map[rune]bool, len(chars))
	missing := make([]rune, 0, len(chars))
	for _, char := range chars {
		if seen[char] {
			continue
		}
		seen[char] = true
		if hasRune(info, char) {
			continue
		}
		missing = append(missing, char)
	}
	return missing
}

func hasRune(info *font.Info, char rune) bool {
	if char < 0 || int(char) >= len(info.UnicodeIndex) {
		return false
	}
	return char == ' ' || info.UnicodeIndex[int(char)] != 0
}

func redrawRunesInCells(lucaFont *font.LucaFont, chars []rune) {
	info := lucaFont.Info
	if info == nil || info.FontFace == nil || lucaFont.Image == nil {
		return
	}
	size := int(info.BlockSize)
	baseline := int(info.FontSize)
	if baseline > size-1 {
		baseline = size - 1
	}
	if baseline < 1 {
		baseline = 1
	}
	clear := &image.Uniform{C: color.NRGBA{}}
	for _, char := range chars {
		if char < 0 || int(char) >= len(info.UnicodeIndex) {
			continue
		}
		index := int(info.UnicodeIndex[int(char)])
		if index == 0 && char != ' ' {
			continue
		}
		rect := image.Rect((index%100)*size, (index/100)*size, (index%100+1)*size, (index/100+1)*size)
		draw.Draw(lucaFont.Image, rect, clear, image.Point{}, draw.Src)
		dot := fixed.Point26_6{
			X: fixed.I(rect.Min.X),
			Y: fixed.I(rect.Min.Y + baseline),
		}
		bounds, glyph, maskPoint, _, ok := info.FontFace.Glyph(dot, char)
		if !ok {
			continue
		}
		draw.Draw(lucaFont.Image, bounds.Intersect(rect), glyph, maskPoint, draw.Over)
	}
}

func uniqueRunes(chars []rune) []rune {
	seen := make(map[rune]bool, len(chars))
	out := make([]rune, 0, len(chars))
	for _, char := range chars {
		if seen[char] {
			continue
		}
		seen[char] = true
		out = append(out, char)
	}
	return out
}

func forceIndexMappings(info *font.Info, chars []rune, startIndex int) {
	endIndex := startIndex + len(chars)
	for codepoint, index := range info.UnicodeIndex {
		if int(index) >= startIndex && int(index) < endIndex {
			info.UnicodeIndex[codepoint] = 0
		}
	}
	for offset, char := range chars {
		index := startIndex + offset
		if index < 0 || index >= len(info.IndexUnicode) {
			continue
		}
		info.IndexUnicode[index] = char
		if char >= 0 && int(char) < len(info.UnicodeIndex) {
			info.UnicodeIndex[int(char)] = uint16(index)
		}
	}
}

func normalizeVerticalMetrics(info *font.Info, chars []rune, yOffset int) {
	lowerY := referenceY(info, []rune{'o', 'a'})
	upperY := referenceY(info, []rune{'O', 'A'})
	normalizeVerticalMetricsTo(info, chars, lowerY, upperY, nil, yOffset)
}

func normalizeVerticalMetricsTo(info *font.Info, chars []rune, lowerY uint8, upperY uint8, originalY map[rune]uint8, yOffset int) {
	for _, char := range chars {
		if char < 0 || int(char) >= len(info.UnicodeIndex) {
			continue
		}
		index := info.UnicodeIndex[int(char)]
		if index == 0 && char != ' ' {
			continue
		}
		if y, ok := originalY[char]; ok {
			info.DrawSize[index].Y = addSignedYOffset(y, yOffset)
		} else if unicode.IsUpper(char) {
			info.DrawSize[index].Y = addSignedYOffset(upperY, yOffset)
		} else {
			info.DrawSize[index].Y = addSignedYOffset(lowerY, yOffset)
		}
	}
}

func originalYByRune(info *font.Info, chars []rune) map[rune]uint8 {
	out := make(map[rune]uint8, len(chars))
	for _, char := range chars {
		if char < 0 || int(char) >= len(info.UnicodeIndex) {
			continue
		}
		index := info.UnicodeIndex[int(char)]
		if index == 0 && char != ' ' {
			continue
		}
		if int(index) >= len(info.DrawSize) {
			continue
		}
		out[char] = info.DrawSize[index].Y
	}
	return out
}

func offsetVerticalMetrics(info *font.Info, chars []rune, yOffset int) {
	seen := make(map[uint16]bool, len(chars))
	for _, char := range chars {
		if char < 0 || int(char) >= len(info.UnicodeIndex) {
			continue
		}
		index := info.UnicodeIndex[int(char)]
		if index == 0 && char != ' ' {
			continue
		}
		if int(index) >= len(info.DrawSize) || seen[index] {
			continue
		}
		seen[index] = true
		info.DrawSize[index].Y = addSignedYOffset(info.DrawSize[index].Y, yOffset)
	}
}

func addSignedYOffset(raw uint8, offset int) uint8 {
	value := int(int8(raw)) + offset
	if value < -128 {
		value = -128
	}
	if value > 127 {
		value = 127
	}
	return uint8(int8(value))
}

func referenceY(info *font.Info, candidates []rune) uint8 {
	for _, char := range candidates {
		if char < 0 || int(char) >= len(info.UnicodeIndex) {
			continue
		}
		index := info.UnicodeIndex[int(char)]
		if index == 0 && char != ' ' {
			continue
		}
		return info.DrawSize[index].Y
	}
	return 0
}

func check(err error) {
	if err != nil {
		fatalf("%v", err)
	}
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
