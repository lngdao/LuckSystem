package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/go-restruct/restruct"
	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"lucksystem/charset"
	"lucksystem/font"
	"lucksystem/pak"
)

type mark int

const (
	markAcute mark = iota
	markGrave
	markHook
	markTilde
	markDotBelow
	markCircumflex
	markBreve
	markHorn
	markStroke
)

type composite struct {
	Base  rune
	Marks []mark
}

type markRenderer struct {
	Face xfont.Face
	Size int
}

type sysfontPair struct {
	InfoIndex  int
	GlyphIndex int
}

func main() {
	restruct.EnableExprBeta()

	yOffset := flag.Int("yoffset", 0, "signed y offset applied to composed glyph metrics")
	sysfontMode := flag.Bool("sysfont", false, "patch a combined SYSFONT.PAK instead of split info/glyph paks")
	markFont := flag.String("markfont", "", "optional TTF/OTF used to render Vietnamese marks")
	flag.Parse()

	renderer := loadMarkRenderer(*markFont, 44)
	if *sysfontMode {
		runSysfont(*yOffset, renderer)
		return
	}

	if flag.NArg() < 4 {
		fatalf("usage: compositevietfont [-yoffset N] <info-pak> <charset-file> <output-dir> <glyph-pak> [glyph-pak...]\n       compositevietfont -sysfont [-yoffset N] <SYSFONT.PAK> <charset-file> <output-pak>")
	}

	infoPakName := flag.Arg(0)
	charsetFile := flag.Arg(1)
	outputDir := flag.Arg(2)
	glyphPakNames := flag.Args()[3:]
	check(os.MkdirAll(outputDir, 0755))

	chars := loadComposableChars(charsetFile)

	infoPak := pak.LoadPak(infoPakName, charset.UTF_8)
	infoPak.ReadAll()
	if len(infoPak.Files) == 0 {
		fatalf("empty info pak: %s", infoPakName)
	}

	baseCount := uint16(0)
	for _, entry := range infoPak.Files {
		info := font.LoadFontInfo(entry.Data)
		if int(info.CharNum) >= len(chars) && (baseCount == 0 || info.CharNum < baseCount) {
			baseCount = info.CharNum
		}
	}
	if baseCount == 0 {
		fatalf("%s has no table large enough for %d chars", infoPakName, len(chars))
	}
	startIndex := int(baseCount) - len(chars)
	fmt.Printf("composing %d chars at index %d\n", len(chars), startIndex)

	var patchedInfos [][]byte
	for _, glyphPakName := range glyphPakNames {
		glyphPak := pak.LoadPak(glyphPakName, charset.UTF_8)
		glyphPak.ReadAll()
		if len(glyphPak.Files) != len(infoPak.Files) {
			fatalf("file count mismatch: %s has %d, %s has %d",
				glyphPakName, len(glyphPak.Files), infoPakName, len(infoPak.Files))
		}

		for index, glyphEntry := range glyphPak.Files {
			infoEntry := infoPak.Files[index]
			lucaFont := font.LoadLucaFont(infoEntry.Data, glyphEntry.Data)
			composeFont(lucaFont, chars, startIndex, *yOffset, renderer)

			var glyphOut bytes.Buffer
			var infoOut bytes.Buffer
			check(lucaFont.Write(&glyphOut, &infoOut))
			check(glyphPak.Set(glyphEntry.Name, bytes.NewReader(glyphOut.Bytes())))
			if patchedInfos == nil {
				patchedInfos = make([][]byte, len(infoPak.Files))
			}
			if patchedInfos[index] == nil {
				patchedInfos[index] = append([]byte(nil), infoOut.Bytes()...)
			}
		}

		glyphPak.Rebuild = true
		outName := filepath.Join(outputDir, trimOG(filepath.Base(glyphPakName)))
		out, err := os.Create(outName)
		check(err)
		check(glyphPak.Write(out))
		check(out.Close())
		fmt.Printf("wrote %s\n", outName)
	}

	for index, infoBytes := range patchedInfos {
		if infoBytes == nil {
			fatalf("missing patched info at index %d", index)
		}
		check(infoPak.Set(infoPak.Files[index].Name, bytes.NewReader(infoBytes)))
	}
	infoPak.Rebuild = true
	outInfoName := filepath.Join(outputDir, trimOG(filepath.Base(infoPakName)))
	outInfo, err := os.Create(outInfoName)
	check(err)
	check(infoPak.Write(outInfo))
	check(outInfo.Close())
	fmt.Printf("wrote %s\n", outInfoName)
}

func runSysfont(yOffset int, renderer *markRenderer) {
	if flag.NArg() != 3 {
		fatalf("usage: compositevietfont -sysfont [-yoffset N] <SYSFONT.PAK> <charset-file> <output-pak>")
	}

	src := flag.Arg(0)
	charsetFile := flag.Arg(1)
	outName := flag.Arg(2)
	chars := loadComposableChars(charsetFile)

	p := pak.LoadPak(src, charset.UTF_8)
	p.ReadAll()
	pairs := []sysfontPair{
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
		startIndex := int(lucaFont.Info.CharNum) - len(chars)
		if startIndex < 0 {
			fatalf("%s has only %d cells for %d chars", infoEntry.Name, lucaFont.Info.CharNum, len(chars))
		}
		composeFont(lucaFont, chars, startIndex, yOffset, renderer)

		var glyphOut bytes.Buffer
		var infoOut bytes.Buffer
		check(lucaFont.Write(&glyphOut, &infoOut))
		check(p.SetByIndex(current.InfoIndex, bytes.NewReader(infoOut.Bytes())))
		check(p.SetByIndex(current.GlyphIndex, bytes.NewReader(glyphOut.Bytes())))
		fmt.Printf("composed %d chars in %s/%s at index %d\n", len(chars), infoEntry.Name, glyphEntry.Name, startIndex)
	}

	p.Rebuild = true
	out, err := os.Create(outName)
	check(err)
	check(p.Write(out))
	check(out.Close())
	fmt.Printf("wrote %s\n", outName)
}

func loadComposableChars(charsetFile string) []rune {
	charsBytes, err := os.ReadFile(charsetFile)
	check(err)
	requested := uniqueRunes([]rune(strings.TrimRight(strings.TrimPrefix(string(charsBytes), "\ufeff"), "\r\n")))
	chars := make([]rune, 0, len(requested))
	for _, char := range requested {
		if _, ok := vietnameseComposite(char); ok {
			chars = append(chars, char)
		}
	}
	if len(chars) == 0 {
		fatalf("charset has no composable Vietnamese characters")
	}
	return chars
}

func loadMarkRenderer(fontFile string, defaultSize int) *markRenderer {
	if fontFile == "" {
		return nil
	}
	f, err := os.Open(fontFile)
	check(err)
	defer f.Close()
	data, err := io.ReadAll(f)
	check(err)
	ttf, err := opentype.Parse(data)
	check(err)
	face, err := opentype.NewFace(ttf, &opentype.FaceOptions{
		Size: float64(defaultSize),
		DPI:  72,
	})
	check(err)
	return &markRenderer{Face: face, Size: defaultSize}
}

func composeFont(lucaFont *font.LucaFont, chars []rune, startIndex int, yOffset int, renderer *markRenderer) {
	info := lucaFont.Info
	size := int(info.BlockSize)
	clearPatchMappings(info, startIndex, len(chars))

	for offset, char := range chars {
		spec, ok := vietnameseComposite(char)
		if !ok {
			continue
		}
		if spec.Base < 0 || int(spec.Base) >= len(info.UnicodeIndex) {
			continue
		}
		sourceIndex := int(info.UnicodeIndex[int(spec.Base)])
		if sourceIndex == 0 && spec.Base != ' ' {
			continue
		}
		targetIndex := startIndex + offset
		if targetIndex < 0 || targetIndex >= int(info.CharNum) || sourceIndex >= int(info.CharNum) {
			continue
		}

		sourceRect := cellRect(sourceIndex, size)
		targetRect := cellRect(targetIndex, size)
		topShift := topRoomShift(size, spec.Marks)
		if spec.Base == 'y' || spec.Base == 'Y' {
			topShift = 0
		}
		copyCell(lucaFont.Image, sourceRect, targetRect, topShift)
		bounds := alphaBounds(lucaFont.Image, targetRect)
		if spec.Base == 'i' && hasTopMark(spec.Marks) {
			eraseTopDetachedComponent(lucaFont.Image, targetRect, bounds)
			bounds = alphaBounds(lucaFont.Image, targetRect)
		}
		drawMarks(lucaFont.Image, targetRect, bounds, char, spec.Marks, renderer)

		info.IndexUnicode[targetIndex] = char
		info.UnicodeIndex[int(char)] = uint16(targetIndex)
		info.DrawSize[targetIndex] = info.DrawSize[sourceIndex]
		if topShift != 0 {
			info.DrawSize[targetIndex].Y = signedByte(signedInt(info.DrawSize[targetIndex].Y) - topShift)
		}
		if yOffset != 0 {
			info.DrawSize[targetIndex].Y = signedByte(signedInt(info.DrawSize[targetIndex].Y) + yOffset)
		}
		info.UnicodeSize[int(char)] = info.UnicodeSize[int(spec.Base)]
	}
}

func clearPatchMappings(info *font.Info, startIndex int, count int) {
	endIndex := startIndex + count
	for codepoint, index := range info.UnicodeIndex {
		if int(index) >= startIndex && int(index) < endIndex {
			info.UnicodeIndex[codepoint] = 0
		}
	}
	for index := startIndex; index < endIndex && index < len(info.IndexUnicode); index++ {
		info.IndexUnicode[index] = 0
	}
}

func hasTopMark(marks []mark) bool {
	for _, current := range marks {
		switch current {
		case markAcute, markGrave, markHook, markTilde, markCircumflex, markBreve:
			return true
		}
	}
	return false
}

func topRoomShift(size int, marks []mark) int {
	count := 0
	hasBottomDot := false
	for _, current := range marks {
		switch current {
		case markAcute, markGrave, markHook, markTilde, markCircumflex, markBreve:
			count++
		case markDotBelow:
			hasBottomDot = true
		}
	}
	if count == 0 {
		return 0
	}
	if hasBottomDot {
		return max(1, size/18)
	}
	if count == 1 {
		return max(2, size/9)
	}
	return max(4, size/5)
}

func copyCell(img *image.NRGBA, source image.Rectangle, target image.Rectangle, yShift int) {
	for y := 0; y < target.Dy(); y++ {
		for x := 0; x < target.Dx(); x++ {
			img.SetNRGBA(target.Min.X+x, target.Min.Y+y, color.NRGBA{})
		}
	}
	for y := 0; y < source.Dy() && y < target.Dy(); y++ {
		dstY := target.Min.Y + y + yShift
		if dstY < target.Min.Y || dstY >= target.Max.Y {
			continue
		}
		for x := 0; x < source.Dx() && x < target.Dx(); x++ {
			img.SetNRGBA(target.Min.X+x, dstY, img.NRGBAAt(source.Min.X+x, source.Min.Y+y))
		}
	}
}

func cellRect(index int, size int) image.Rectangle {
	x := (index % 100) * size
	y := (index / 100) * size
	return image.Rect(x, y, x+size, y+size)
}

func alphaBounds(img *image.NRGBA, rect image.Rectangle) image.Rectangle {
	minX, minY := rect.Max.X, rect.Max.Y
	maxX, maxY := rect.Min.X-1, rect.Min.Y-1
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if img.NRGBAAt(x, y).A < 24 {
				continue
			}
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return rect
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func drawMarks(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle, char rune, marks []mark, renderer *markRenderer) {
	if len(marks) == 0 {
		return
	}
	size := cell.Dx()
	thick := max(1, size/30)
	gap := max(3, size/9)
	hasBottomDot := containsMark(marks, markDotBelow)
	if hasBottomDot {
		gap = 1
	}
	ink := sampleInk(img, glyph)
	topLevel := 0
	for _, current := range marks {
		switch current {
		case markAcute, markGrave, markHook, markTilde, markCircumflex, markBreve:
			if renderer == nil || !drawTTFTopMark(img, cell, glyph, current, topLevel, gap, ink, renderer) {
				drawTopMark(img, cell, glyph, current, topLevel, thick, gap, ink)
			}
			topLevel++
		case markDotBelow:
			if renderer == nil || !drawTTFDotBelow(img, cell, glyph, gap, hasBottomDot && topLevel > 0, ink, renderer) {
				drawDotBelow(img, cell, glyph, thick, gap, hasBottomDot && topLevel > 0, ink)
			}
		case markHorn:
			if renderer == nil || !drawTTFHorn(img, cell, glyph, char, ink, renderer) {
				drawHorn(img, cell, glyph, thick, ink)
			}
		case markStroke:
			drawStroke(img, cell, glyph, unicode.IsUpper(char), thick, ink)
		}
	}
}

func drawTTFTopMark(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle, current mark, level int, gap int, ink color.NRGBA, renderer *markRenderer) bool {
	markRune, ok := ttfTopMarkRune(current)
	if !ok {
		return false
	}
	markImg, bounds, ok := renderMarkGlyph(renderer, markRune)
	if !ok {
		return false
	}
	centerX := (glyph.Min.X + glyph.Max.X) / 2
	x := centerX - bounds.Dx()/2
	y := glyph.Min.Y - gap - bounds.Dy() - level*(bounds.Dy()+max(2, gap/2))
	if y < cell.Min.Y+1 {
		y = cell.Min.Y + 1
	}
	blitMark(img, markImg, image.Pt(x, y), cell, ink)
	return true
}

func drawTTFDotBelow(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle, gap int, hasTopMark bool, ink color.NRGBA, renderer *markRenderer) bool {
	markImg, bounds, ok := renderMarkGlyph(renderer, '\u0323')
	if !ok {
		return false
	}
	centerX := (glyph.Min.X + glyph.Max.X) / 2
	extraGap := max(3, gap)
	if hasTopMark {
		extraGap = max(4, gap+2)
	}
	x := centerX - bounds.Dx()/2
	y := glyph.Max.Y + extraGap
	if y+bounds.Dy() >= cell.Max.Y {
		y = cell.Max.Y - bounds.Dy() - 1
	}
	blitMark(img, markImg, image.Pt(x, y), cell, ink)
	return true
}

func drawTTFHorn(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle, char rune, ink color.NRGBA, renderer *markRenderer) bool {
	markImg, bounds, ok := renderMarkGlyph(renderer, '\u031b')
	if !ok {
		return false
	}
	x := glyph.Max.X - bounds.Dx()/2
	if unicode.ToLower(char) == 'ư' {
		x += max(1, cell.Dx()/18)
	}
	y := glyph.Min.Y - max(2, cell.Dx()/10)
	if x+bounds.Dx() >= cell.Max.X {
		x = cell.Max.X - bounds.Dx() - 1
	}
	if y < cell.Min.Y {
		y = cell.Min.Y
	}
	blitMark(img, markImg, image.Pt(x, y), cell, ink)
	return true
}

func ttfTopMarkRune(current mark) (rune, bool) {
	switch current {
	case markAcute:
		return '\u0301', true
	case markGrave:
		return '\u0300', true
	case markHook:
		return '\u0309', true
	case markTilde:
		return '\u0303', true
	case markCircumflex:
		return '\u0302', true
	case markBreve:
		return '\u0306', true
	default:
		return 0, false
	}
}

func renderMarkGlyph(renderer *markRenderer, char rune) (*image.Alpha, image.Rectangle, bool) {
	if renderer == nil || renderer.Face == nil {
		return nil, image.Rectangle{}, false
	}
	canvasSize := max(96, renderer.Size*3)
	dst := image.NewAlpha(image.Rect(0, 0, canvasSize, canvasSize))
	dot := fixed.Point26_6{
		X: fixed.I(canvasSize / 2),
		Y: fixed.I((canvasSize * 2) / 3),
	}
	dr, mask, maskp, _, ok := renderer.Face.Glyph(dot, char)
	if !ok || dr.Empty() {
		return nil, image.Rectangle{}, false
	}
	draw.Draw(dst, dr.Intersect(dst.Bounds()), mask, maskp, draw.Src)
	bounds := alphaMaskBounds(dst)
	if bounds.Empty() {
		return nil, image.Rectangle{}, false
	}
	crop := image.NewAlpha(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(crop, crop.Bounds(), dst, bounds.Min, draw.Src)
	return crop, crop.Bounds(), true
}

func alphaMaskBounds(img *image.Alpha) image.Rectangle {
	minX, minY := img.Bounds().Max.X, img.Bounds().Max.Y
	maxX, maxY := img.Bounds().Min.X-1, img.Bounds().Min.Y-1
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			if img.AlphaAt(x, y).A < 16 {
				continue
			}
			if x < minX {
				minX = x
			}
			if y < minY {
				minY = y
			}
			if x > maxX {
				maxX = x
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if maxX < minX || maxY < minY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

func blitMark(dst *image.NRGBA, src *image.Alpha, at image.Point, clip image.Rectangle, ink color.NRGBA) {
	for y := 0; y < src.Bounds().Dy(); y++ {
		for x := 0; x < src.Bounds().Dx(); x++ {
			p := image.Pt(at.X+x, at.Y+y)
			if !p.In(clip) || !p.In(dst.Bounds()) {
				continue
			}
			a := src.AlphaAt(x, y).A
			if a == 0 {
				continue
			}
			outA := uint8((int(a) * int(ink.A)) / 255)
			existing := dst.NRGBAAt(p.X, p.Y)
			if existing.A > outA {
				outA = existing.A
			}
			dst.SetNRGBA(p.X, p.Y, color.NRGBA{R: ink.R, G: ink.G, B: ink.B, A: outA})
		}
	}
}

func containsMark(marks []mark, wanted mark) bool {
	for _, current := range marks {
		if current == wanted {
			return true
		}
	}
	return false
}

func sampleInk(img *image.NRGBA, glyph image.Rectangle) color.NRGBA {
	for y := glyph.Min.Y; y < glyph.Max.Y; y++ {
		for x := glyph.Min.X; x < glyph.Max.X; x++ {
			c := img.NRGBAAt(x, y)
			if c.A >= 128 {
				return color.NRGBA{R: c.R, G: c.G, B: c.B, A: 230}
			}
		}
	}
	return color.NRGBA{A: 230}
}

func drawTopMark(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle, current mark, level int, thick int, gap int, ink color.NRGBA) {
	size := cell.Dx()
	centerX := (glyph.Min.X + glyph.Max.X) / 2
	width := max(4, size/5)
	height := max(3, size/9)
	levelGap := max(3, height+gap/2)
	baseY := glyph.Min.Y - gap - level*levelGap
	if baseY-height < cell.Min.Y+1 {
		baseY = cell.Min.Y + 1 + height
	}

	switch current {
	case markAcute:
		line(img, centerX-width/3, baseY, centerX+width/3, baseY-height, thick, ink)
	case markGrave:
		line(img, centerX-width/3, baseY-height, centerX+width/3, baseY, thick, ink)
	case markHook:
		hookW := max(5, width*3/4)
		hookH := max(5, height+2)
		x := centerX - hookW/4
		y := baseY - hookH
		line(img, x, y, x+hookW/3, y, thick, ink)
		line(img, x+hookW/3, y, x+hookW/2, y+hookH/3, thick, ink)
		line(img, x+hookW/2, y+hookH/3, centerX, baseY-max(1, hookH/4), thick, ink)
		softDot(img, centerX, baseY+max(1, thick), max(1, thick), ink)
	case markTilde:
		y := baseY - height/2
		line(img, centerX-width/2, y, centerX-width/6, y-1, thick, ink)
		line(img, centerX-width/6, y-1, centerX+width/6, y+1, thick, ink)
		line(img, centerX+width/6, y+1, centerX+width/2, y, thick, ink)
	case markCircumflex:
		line(img, centerX-width/2, baseY, centerX, baseY-height, thick, ink)
		line(img, centerX, baseY-height, centerX+width/2, baseY, thick, ink)
	case markBreve:
		line(img, centerX-width/2, baseY-height, centerX-width/4, baseY, thick, ink)
		line(img, centerX-width/4, baseY, centerX+width/4, baseY, thick, ink)
		line(img, centerX+width/4, baseY, centerX+width/2, baseY-height, thick, ink)
	}
}

func drawDotBelow(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle, thick int, gap int, hasTopMark bool, ink color.NRGBA) {
	centerX := (glyph.Min.X + glyph.Max.X) / 2
	radius := max(2, thick+2)
	extraGap := max(3, gap)
	if hasTopMark {
		extraGap = max(4, gap+2)
	}
	y := glyph.Max.Y + extraGap + radius
	if y >= cell.Max.Y-radius-1 {
		y = cell.Max.Y - radius - 1
	}
	softDot(img, centerX, y, radius, ink)
}

func drawHorn(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle, thick int, ink color.NRGBA) {
	size := cell.Dx()
	hornThick := thick + 1
	x := min(cell.Max.X-4, glyph.Max.X-max(1, size/18))
	y := glyph.Min.Y + max(5, size/4)
	x1 := min(cell.Max.X-3, x+max(4, size/8))
	y1 := max(cell.Min.Y+2, y-max(4, size/8))
	x2 := min(cell.Max.X-3, x1+max(2, size/18))
	y2 := min(cell.Max.Y-3, y+max(5, size/7))
	line(img, x, y, x1, y1, hornThick, ink)
	line(img, x1, y1, x2, y1+max(2, size/20), hornThick, ink)
	line(img, x2, y1+max(2, size/20), x2-max(1, size/24), y2, hornThick, ink)
}

func drawStroke(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle, upper bool, thick int, ink color.NRGBA) {
	y := glyph.Min.Y + (glyph.Dy() * 30 / 100)
	x0 := glyph.Min.X + glyph.Dx()*34/100
	x1 := glyph.Min.X + glyph.Dx()*68/100
	if upper {
		y = glyph.Min.Y + (glyph.Dy() * 47 / 100)
		x0 = glyph.Min.X + glyph.Dx()*0/100
		x1 = glyph.Min.X + glyph.Dx()*54/100
	} else {
		y = glyph.Min.Y + (glyph.Dy() * 22 / 100)
		x0 = glyph.Min.X + glyph.Dx()*54/100
		x1 = glyph.Min.X + glyph.Dx()*96/100
	}
	line(img, x0, y, min(cell.Max.X-2, x1), y, max(1, thick), ink)
}

func eraseTopDetachedComponent(img *image.NRGBA, cell image.Rectangle, glyph image.Rectangle) {
	rowInk := make([]bool, cell.Dy())
	firstInk := -1
	for y := glyph.Min.Y; y < glyph.Max.Y; y++ {
		for x := glyph.Min.X; x < glyph.Max.X; x++ {
			if img.NRGBAAt(x, y).A < 24 {
				continue
			}
			row := y - cell.Min.Y
			rowInk[row] = true
			if firstInk < 0 {
				firstInk = row
			}
			break
		}
	}
	if firstInk < 0 {
		return
	}
	lastDotRow := -1
	seenInk := false
	for row := firstInk; row < len(rowInk); row++ {
		if rowInk[row] {
			seenInk = true
			lastDotRow = row
			continue
		}
		if seenInk {
			break
		}
	}
	if lastDotRow < firstInk {
		return
	}
	eraseMaxY := cell.Min.Y + lastDotRow + 1
	for y := cell.Min.Y + firstInk; y < eraseMaxY; y++ {
		for x := glyph.Min.X; x < glyph.Max.X; x++ {
			img.SetNRGBA(x, y, color.NRGBA{})
		}
	}
}

func line(img *image.NRGBA, x0 int, y0 int, x1 int, y1 int, thick int, ink color.NRGBA) {
	dx := abs(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -abs(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		dot(img, x0, y0, thick, ink)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func dot(img *image.NRGBA, cx int, cy int, radius int, ink color.NRGBA) {
	for y := cy - radius; y <= cy+radius; y++ {
		for x := cx - radius; x <= cx+radius; x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) > radius*radius {
				continue
			}
			existing := img.NRGBAAt(x, y)
			a := uint8(230)
			if existing.A > a {
				a = existing.A
			}
			img.SetNRGBA(x, y, color.NRGBA{R: ink.R, G: ink.G, B: ink.B, A: a})
		}
	}
}

func softDot(img *image.NRGBA, cx int, cy int, radius int, ink color.NRGBA) {
	outer := radius + 1
	rr := radius * radius
	oo := outer * outer
	for y := cy - outer; y <= cy+outer; y++ {
		for x := cx - outer; x <= cx+outer; x++ {
			if !image.Pt(x, y).In(img.Bounds()) {
				continue
			}
			d2 := (x-cx)*(x-cx) + (y-cy)*(y-cy)
			if d2 > oo {
				continue
			}
			alpha := int(ink.A)
			if d2 > rr {
				alpha = alpha / 2
			}
			existing := img.NRGBAAt(x, y)
			if int(existing.A) > alpha {
				alpha = int(existing.A)
			}
			img.SetNRGBA(x, y, color.NRGBA{R: ink.R, G: ink.G, B: ink.B, A: uint8(alpha)})
		}
	}
}

func vietnameseComposite(char rune) (composite, bool) {
	lower := unicode.ToLower(char)
	upper := unicode.IsUpper(char)
	spec, ok := vietnameseLower(lower)
	if !ok {
		return composite{}, false
	}
	if upper {
		spec.Base = unicode.ToUpper(spec.Base)
	}
	return spec, true
}

func vietnameseLower(char rune) (composite, bool) {
	m := map[rune]composite{
		'á': {'a', []mark{markAcute}}, 'à': {'a', []mark{markGrave}}, 'ả': {'a', []mark{markHook}}, 'ã': {'a', []mark{markTilde}}, 'ạ': {'a', []mark{markDotBelow}},
		'â': {'a', []mark{markCircumflex}}, 'ấ': {'a', []mark{markCircumflex, markAcute}}, 'ầ': {'a', []mark{markCircumflex, markGrave}}, 'ẩ': {'a', []mark{markCircumflex, markHook}}, 'ẫ': {'a', []mark{markCircumflex, markTilde}}, 'ậ': {'a', []mark{markCircumflex, markDotBelow}},
		'ă': {'a', []mark{markBreve}}, 'ắ': {'a', []mark{markBreve, markAcute}}, 'ằ': {'a', []mark{markBreve, markGrave}}, 'ẳ': {'a', []mark{markBreve, markHook}}, 'ẵ': {'a', []mark{markBreve, markTilde}}, 'ặ': {'a', []mark{markBreve, markDotBelow}},
		'é': {'e', []mark{markAcute}}, 'è': {'e', []mark{markGrave}}, 'ẻ': {'e', []mark{markHook}}, 'ẽ': {'e', []mark{markTilde}}, 'ẹ': {'e', []mark{markDotBelow}},
		'ê': {'e', []mark{markCircumflex}}, 'ế': {'e', []mark{markCircumflex, markAcute}}, 'ề': {'e', []mark{markCircumflex, markGrave}}, 'ể': {'e', []mark{markCircumflex, markHook}}, 'ễ': {'e', []mark{markCircumflex, markTilde}}, 'ệ': {'e', []mark{markCircumflex, markDotBelow}},
		'í': {'i', []mark{markAcute}}, 'ì': {'i', []mark{markGrave}}, 'ỉ': {'i', []mark{markHook}}, 'ĩ': {'i', []mark{markTilde}}, 'ị': {'i', []mark{markDotBelow}},
		'ó': {'o', []mark{markAcute}}, 'ò': {'o', []mark{markGrave}}, 'ỏ': {'o', []mark{markHook}}, 'õ': {'o', []mark{markTilde}}, 'ọ': {'o', []mark{markDotBelow}},
		'ô': {'o', []mark{markCircumflex}}, 'ố': {'o', []mark{markCircumflex, markAcute}}, 'ồ': {'o', []mark{markCircumflex, markGrave}}, 'ổ': {'o', []mark{markCircumflex, markHook}}, 'ỗ': {'o', []mark{markCircumflex, markTilde}}, 'ộ': {'o', []mark{markCircumflex, markDotBelow}},
		'ơ': {'o', []mark{markHorn}}, 'ớ': {'o', []mark{markHorn, markAcute}}, 'ờ': {'o', []mark{markHorn, markGrave}}, 'ở': {'o', []mark{markHorn, markHook}}, 'ỡ': {'o', []mark{markHorn, markTilde}}, 'ợ': {'o', []mark{markHorn, markDotBelow}},
		'ú': {'u', []mark{markAcute}}, 'ù': {'u', []mark{markGrave}}, 'ủ': {'u', []mark{markHook}}, 'ũ': {'u', []mark{markTilde}}, 'ụ': {'u', []mark{markDotBelow}},
		'ư': {'u', []mark{markHorn}}, 'ứ': {'u', []mark{markHorn, markAcute}}, 'ừ': {'u', []mark{markHorn, markGrave}}, 'ử': {'u', []mark{markHorn, markHook}}, 'ữ': {'u', []mark{markHorn, markTilde}}, 'ự': {'u', []mark{markHorn, markDotBelow}},
		'ý': {'y', []mark{markAcute}}, 'ỳ': {'y', []mark{markGrave}}, 'ỷ': {'y', []mark{markHook}}, 'ỹ': {'y', []mark{markTilde}}, 'ỵ': {'y', []mark{markDotBelow}},
		'đ': {'d', []mark{markStroke}},
	}
	spec, ok := m[char]
	return spec, ok
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

func trimOG(name string) string {
	return strings.TrimSuffix(name, "_OG")
}

func signedInt(raw uint8) int { return int(int8(raw)) }

func signedByte(value int) uint8 {
	if value < -128 {
		value = -128
	}
	if value > 127 {
		value = 127
	}
	return uint8(int8(value))
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
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
