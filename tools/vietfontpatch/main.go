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
	"path/filepath"
	"strings"
	"unicode"

	"github.com/go-restruct/restruct"
	"golang.org/x/image/math/fixed"

	"lucksystem/charset"
	"lucksystem/font"
	"lucksystem/pak"
)

type fontSet struct {
	Name       string
	InfoPak    string
	FamilyPaks []string
}

func main() {
	restruct.EnableExprBeta()

	slot := flag.String("slot", "all", "font slot to patch: all, en, zc, hd0/font0")
	family := flag.String("family", "all", "font family to patch: all, GOTHIC1, GOTHIC2, GOTHIC3, MINCHO, MODERN")
	yOffset := flag.Int("yoffset", 0, "extra signed vertical offset for injected characters")
	scale := flag.Float64("scale", 1.0, "temporary TTF render scale relative to each Luca font size")
	forceAll := flag.Bool("forceall", false, "inject every requested character instead of only characters missing from the original table")
	normalizeY := flag.Bool("normalizey", true, "normalize injected lowercase/uppercase y offsets to reference glyphs")
	flag.Parse()

	if flag.NArg() != 4 {
		fatalf("usage: vietfontpatch [-slot all|en|zc] [-family all|GOTHIC1|...] [-yoffset N] <font-root> <charset-file> <ttf-file> <output-dir>")
	}

	fontRoot, err := filepath.Abs(flag.Arg(0))
	check(err)
	charsetFile, err := filepath.Abs(flag.Arg(1))
	check(err)
	ttfFile, err := filepath.Abs(flag.Arg(2))
	check(err)
	outputDir, err := filepath.Abs(flag.Arg(3))
	check(err)
	check(os.MkdirAll(outputDir, 0755))

	charsBytes, err := os.ReadFile(charsetFile)
	check(err)
	chars := strings.TrimPrefix(string(charsBytes), "\ufeff")
	chars = strings.TrimRight(chars, "\r\n")
	if chars == "" {
		fatalf("charset is empty: %s", charsetFile)
	}

	ttfBytes, err := os.ReadFile(ttfFile)
	check(err)

	sets := []fontSet{
		{
			Name:    "jp",
			InfoPak: filepath.Join(fontRoot, "font_win32_1280", "FONT__INFO.PAK"),
			FamilyPaks: []string{
				filepath.Join(fontRoot, "font_win32_1280", "FONT_GOTHIC1.PAK"),
				filepath.Join(fontRoot, "font_win32_1280", "FONT_GOTHIC2.PAK"),
				filepath.Join(fontRoot, "font_win32_1280", "FONT_GOTHIC3.PAK"),
				filepath.Join(fontRoot, "font_win32_1280", "FONT_MINCHO.PAK"),
				filepath.Join(fontRoot, "font_win32_1280", "FONT_MODERN.PAK"),
			},
		},
		{
			Name:    "zc",
			InfoPak: filepath.Join(fontRoot, "fontzc_win32_1280", "FONTZC__INFO.PAK"),
			FamilyPaks: []string{
				filepath.Join(fontRoot, "fontzc_win32_1280", "FONTZC_GOTHIC1.PAK"),
				filepath.Join(fontRoot, "fontzc_win32_1280", "FONTZC_GOTHIC2.PAK"),
				filepath.Join(fontRoot, "fontzc_win32_1280", "FONTZC_MINCHO.PAK"),
			},
		},
		{
			Name:    "hd0",
			InfoPak: filepath.Join(fontRoot, "font_win32_1920", "FONT0__INFO.PAK"),
			FamilyPaks: []string{
				filepath.Join(fontRoot, "font_win32_1920", "FONT0_GOTHIC1.PAK"),
				filepath.Join(fontRoot, "font_win32_1920", "FONT0_GOTHIC2.PAK"),
				filepath.Join(fontRoot, "font_win32_1920", "FONT0_GOTHIC3.PAK"),
				filepath.Join(fontRoot, "font_win32_1920", "FONT0_MINCHO.PAK"),
				filepath.Join(fontRoot, "font_win32_1920", "FONT0_MODERN.PAK"),
			},
		},
	}

	patched := false
	for _, set := range sets {
		if *slot != "all" && *slot != set.Name && !(*slot == "en" && set.Name == "jp") && !((*slot == "font0" || *slot == "hd") && set.Name == "hd0") {
			continue
		}
		set.FamilyPaks = filterFamilyPaks(set.FamilyPaks, *family)
		if len(set.FamilyPaks) == 0 {
			continue
		}
		fmt.Printf("patching %s\n", set.Name)
		check(patchSet(set, chars, ttfBytes, outputDir, *yOffset, *forceAll, *normalizeY, *scale))
		patched = true
	}
	if !patched {
		fatalf("no font family matched slot=%s family=%s", *slot, *family)
	}
}

func filterFamilyPaks(familyPaks []string, family string) []string {
	filter := strings.ToUpper(strings.TrimSuffix(family, ".PAK"))
	if filter == "" || filter == "ALL" {
		return familyPaks
	}
	selected := make([]string, 0, len(familyPaks))
	for _, familyPak := range familyPaks {
		base := strings.ToUpper(strings.TrimSuffix(filepath.Base(familyPak), ".PAK"))
		if base == filter || strings.HasSuffix(base, "_"+filter) || strings.Contains(base, filter) {
			selected = append(selected, familyPak)
		}
	}
	return selected
}

func patchSet(set fontSet, chars string, ttfBytes []byte, outputDir string, yOffset int, forceAll bool, normalizeY bool, scale float64) error {
	infoPak := pak.LoadPak(set.InfoPak, charset.UTF_8)
	infoPak.ReadAll()
	if len(infoPak.Files) == 0 {
		return fmt.Errorf("empty info pak: %s", set.InfoPak)
	}
	requestedRunes := []rune(chars)
	referenceInfo := font.LoadFontInfo(infoPak.Files[0].Data)
	patchRunes := missingRunes(referenceInfo, requestedRunes)
	if forceAll {
		patchRunes = uniqueRunes(requestedRunes)
	}
	patchChars := string(patchRunes)
	if len(patchRunes) == 0 {
		return fmt.Errorf("%s already contains all requested characters", set.InfoPak)
	}

	baseCount := uint16(0)
	for _, entry := range infoPak.Files {
		info := font.LoadFontInfo(entry.Data)
		if int(info.CharNum) >= len(patchRunes) && (baseCount == 0 || info.CharNum < baseCount) {
			baseCount = info.CharNum
		}
	}
	if baseCount == 0 {
		return fmt.Errorf("%s has no info table large enough for %d chars", set.InfoPak, len(patchRunes))
	}
	startIndex := int(baseCount) - len(patchRunes)
	fmt.Printf("  requested: %d, already present: %d, injected: %d\n",
		len(requestedRunes), len(requestedRunes)-len(patchRunes), len(patchRunes))
	fmt.Printf("  base cells: %d, replace index: %d\n", baseCount, startIndex)

	var patchedInfos [][]byte
	for _, familyPakName := range set.FamilyPaks {
		familyPak := pak.LoadPak(familyPakName, charset.UTF_8)
		familyPak.ReadAll()
		if len(familyPak.Files) != len(infoPak.Files) {
			return fmt.Errorf("file count mismatch: %s has %d, %s has %d",
				familyPakName, len(familyPak.Files), set.InfoPak, len(infoPak.Files))
		}

		for index, glyphEntry := range familyPak.Files {
			infoEntry := infoPak.Files[index]
			lucaFont := font.LoadLucaFont(infoEntry.Data, glyphEntry.Data)
			lowerY := referenceY(lucaFont.Info, []rune{'a', 'o'})
			upperY := referenceY(lucaFont.Info, []rune{'A', 'O'})
			originalY := originalYByRune(lucaFont.Info, patchRunes)
			originalFontSize := lucaFont.Info.FontSize
			if scale > 0 && scale != 1 {
				scaled := int(math.Round(float64(originalFontSize) * scale))
				if scaled < 1 {
					scaled = 1
				}
				lucaFont.Info.FontSize = uint16(scaled)
			}
			lucaFont.ReplaceChars(bytes.NewReader(ttfBytes), patchChars, startIndex, false)
			redrawRunesInCells(lucaFont, patchRunes)
			lucaFont.Info.FontSize = originalFontSize
			forceIndexMappings(lucaFont.Info, patchRunes, startIndex)
			if normalizeY {
				normalizeVerticalMetricsTo(lucaFont.Info, patchRunes, lowerY, upperY, originalY, yOffset)
			} else if yOffset != 0 {
				offsetVerticalMetrics(lucaFont.Info, patchRunes, yOffset)
			}

			var glyphOut bytes.Buffer
			var infoOut bytes.Buffer
			if err := lucaFont.Write(&glyphOut, &infoOut); err != nil {
				return fmt.Errorf("%s/%s: %w", familyPakName, glyphEntry.Name, err)
			}
			if err := familyPak.Set(glyphEntry.Name, bytes.NewReader(glyphOut.Bytes())); err != nil {
				return err
			}
			if patchedInfos == nil {
				patchedInfos = make([][]byte, len(infoPak.Files))
			}
			if patchedInfos[index] == nil {
				patchedInfos[index] = append([]byte(nil), infoOut.Bytes()...)
			}
		}
		familyPak.Rebuild = true

		outName := filepath.Join(outputDir, filepath.Base(familyPakName))
		outFile, err := os.Create(outName)
		if err != nil {
			return err
		}
		if err := familyPak.Write(outFile); err != nil {
			_ = outFile.Close()
			return err
		}
		check(outFile.Close())
		fmt.Printf("  wrote %s\n", outName)
	}

	for index, infoBytes := range patchedInfos {
		if infoBytes == nil {
			return fmt.Errorf("missing patched info at index %d", index)
		}
		if err := infoPak.Set(infoPak.Files[index].Name, bytes.NewReader(infoBytes)); err != nil {
			return err
		}
	}
	outInfoName := filepath.Join(outputDir, filepath.Base(set.InfoPak))
	outInfo, err := os.Create(outInfoName)
	if err != nil {
		return err
	}
	if err := infoPak.Write(outInfo); err != nil {
		_ = outInfo.Close()
		return err
	}
	check(outInfo.Close())
	fmt.Printf("  wrote %s\n", outInfoName)
	return nil
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
	for unicode, index := range info.UnicodeIndex {
		if int(index) >= startIndex && int(index) < endIndex {
			info.UnicodeIndex[unicode] = 0
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

func normalizeVerticalMetrics(info *font.Info, chars []rune, yOffset int) {
	lowerY := referenceY(info, []rune{'ó', 'á', 'a', 'o'})
	upperY := referenceY(info, []rune{'Á', 'Â', 'A', 'O'})
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
