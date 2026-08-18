// Package ogimage renders the fixed-layout Open Graph preview card used by pages
// that carry no page-specific card of their own (currently /open and /about):
// the freehire wordmark, a heading and tagline, and a strip of catalogue-scale
// figures. 1200x630, matching the size the site's other cards (rendered
// separately in web/src/lib/server/og, for jobs/companies/blog posts) already
// use.
//
// This package takes a plain Stats value rather than a catalogstats.Result, so
// it stays testable and reusable without importing the snapshot-loading
// machinery — internal/handler is the one place that bridges the two.
package ogimage

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	width  = 1200
	height = 630

	marginX = 72
)

var (
	colorWhite = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	colorInk   = color.RGBA{R: 0x0a, G: 0x0a, B: 0x0a, A: 0xff}
	colorMuted = color.RGBA{R: 0xa3, G: 0xa3, B: 0xa3, A: 0xff}
)

//go:embed fonts/Inter-Regular.ttf
var interRegularTTF []byte

//go:embed fonts/Inter-Bold.ttf
var interBoldTTF []byte

var regularFont, boldFont *opentype.Font

func init() {
	var err error
	if regularFont, err = opentype.Parse(interRegularTTF); err != nil {
		panic(fmt.Sprintf("ogimage: parsing Inter-Regular: %v", err))
	}
	if boldFont, err = opentype.Parse(interBoldTTF); err != nil {
		panic(fmt.Sprintf("ogimage: parsing Inter-Bold: %v", err))
	}
}

// Stats is the subset of a catalogstats.Result a catalogue card needs: the
// exact open-jobs/companies counts when available, the always-available
// registry-derived source count, and whether the counts are exact or the
// degraded estimate.
type Stats struct {
	OpenJobs  int64
	Companies int64
	Sources   int
	Exact     bool
}

// RenderCatalogCard renders the 1200x630 catalogue-scale OG card: the freehire
// wordmark, heading and tagline, and a strip of catalogue-scale figures drawn
// from stats.
//
// When stats is not exact, the companies figure is omitted rather than shown
// as zero — the degraded snapshot leaves it unset — and the open-jobs figure
// is marked as an estimate, mirroring how catalogstats itself distinguishes a
// measurement from a fallback.
func RenderCatalogCard(heading, tagline string, stats Stats) ([]byte, error) {
	wordmarkFace, err := newFace(boldFont, 32)
	if err != nil {
		return nil, fmt.Errorf("ogimage: wordmark face: %w", err)
	}
	defer wordmarkFace.Close()
	headingFace, err := newFace(boldFont, 52)
	if err != nil {
		return nil, fmt.Errorf("ogimage: heading face: %w", err)
	}
	defer headingFace.Close()
	taglineFace, err := newFace(regularFont, 26)
	if err != nil {
		return nil, fmt.Errorf("ogimage: tagline face: %w", err)
	}
	defer taglineFace.Close()
	numberFace, err := newFace(boldFont, 48)
	if err != nil {
		return nil, fmt.Errorf("ogimage: number face: %w", err)
	}
	defer numberFace.Close()
	labelFace, err := newFace(regularFont, 22)
	if err != nil {
		return nil, fmt.Errorf("ogimage: label face: %w", err)
	}
	defer labelFace.Close()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(colorWhite), image.Point{}, draw.Src)

	drawText(img, wordmarkFace, colorInk, marginX, 96, "freehire")

	const (
		headingLineHeight = 62
		taglineLineHeight = 34
		blockGap          = 8
	)
	y := 200
	for _, line := range wrapLines(headingFace, heading, width-2*marginX) {
		drawText(img, headingFace, colorInk, marginX, y, line)
		y += headingLineHeight
	}
	y += blockGap
	for _, line := range wrapLines(taglineFace, tagline, width-2*marginX) {
		drawText(img, taglineFace, colorMuted, marginX, y, line)
		y += taglineLineHeight
	}

	drawStatStrip(img, numberFace, labelFace, stats)

	footer := "freehire.me"
	drawText(img, labelFace, colorMuted, width-marginX-textWidth(labelFace, footer), 566, footer)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("ogimage: encoding png: %w", err)
	}
	return buf.Bytes(), nil
}

// statChip is one number+label pair in the stat strip.
type statChip struct {
	value string
	label string
}

// chips builds the stat strip for stats. The degraded (non-exact) path omits
// companies rather than render the zero the snapshot leaves it at, and marks
// the open-jobs figure as an estimate.
func chips(s Stats) []statChip {
	if !s.Exact {
		return []statChip{
			{value: "~" + groupThousands(s.OpenJobs), label: "open jobs, estimated"},
			{value: strconv.Itoa(s.Sources), label: "sources"},
		}
	}
	return []statChip{
		{value: groupThousands(s.OpenJobs), label: "open jobs"},
		{value: groupThousands(s.Companies), label: "companies"},
		{value: strconv.Itoa(s.Sources), label: "sources"},
	}
}

func drawStatStrip(img *image.RGBA, numberFace, labelFace font.Face, s Stats) {
	const (
		gap     = 64
		numberY = 520
		labelY  = 552
	)
	x := marginX
	for _, ch := range chips(s) {
		drawText(img, numberFace, colorInk, x, numberY, ch.value)
		drawText(img, labelFace, colorMuted, x, labelY, ch.label)

		w := textWidth(numberFace, ch.value)
		if lw := textWidth(labelFace, ch.label); lw > w {
			w = lw
		}
		x += w + gap
	}
}

func newFace(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

func drawText(img *image.RGBA, f font.Face, c color.Color, x, y int, s string) {
	d := &font.Drawer{Dst: img, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func textWidth(f font.Face, s string) int {
	return (&font.Drawer{Face: f}).MeasureString(s).Round()
}

// wrapLines greedily wraps s to lines no wider than maxWidth, measured in f.
// Fixed-content headings/taglines are short enough that this never needs a
// line cap — the card's vertical layout gives it room.
func wrapLines(f font.Face, s string, maxWidth int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	lines := make([]string, 0, 2)
	line := words[0]
	for _, w := range words[1:] {
		candidate := line + " " + w
		if textWidth(f, candidate) <= maxWidth {
			line = candidate
			continue
		}
		lines = append(lines, line)
		line = w
	}
	return append(lines, line)
}

// groupThousands formats n with comma thousands separators, e.g. 3300658 ->
// "3,300,658". Go's stdlib has no built-in for this, and pulling in
// golang.org/x/text/message for one formatter would be a heavier dependency
// than this handful of lines.
func groupThousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
