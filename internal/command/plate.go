package command

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func pickRandomRegion() string {
	keys := make([]string, 0, len(plateConfigs))
	for k := range plateConfigs {
		keys = append(keys, k)
	}
	return keys[rand.Intn(len(keys))]
}

func generateFormattedNumber(region string) string {
	format := plateConfigs[region].Format
	var result []byte
	for i := 0; i < len(format); i++ {
		ch := format[i]
		switch {
		case ch >= 'A' && ch <= 'Z':
			letters := "ABCDEFGHJKLMNPQRSTUVWXYZ"
			result = append(result, letters[rand.Intn(len(letters))])
		case ch >= '0' && ch <= '9':
			result = append(result, byte('0'+rand.Intn(10)))
		default:
			result = append(result, ch)
		}
	}
	return string(result)
}

func parseHexColor(hex string) color.RGBA {
	if hex[0] == '#' {
		hex = hex[1:]
	}
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	r, _ := strconv.ParseUint(hex[0:2], 16, 8)
	g, _ := strconv.ParseUint(hex[2:4], 16, 8)
	b, _ := strconv.ParseUint(hex[4:6], 16, 8)
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}

var markerColor = parseHexColor("#FF00FF")

func isMarker(img *image.RGBA, x, y int) bool {
	r, g, b, a := img.At(x, y).RGBA()
	mr, mg, mb, ma := markerColor.RGBA()
	return r == mr && g == mg && b == mb && a == ma
}

func findMarkerRegions(img *image.RGBA) []image.Rectangle {
	bounds := img.Bounds()

	// collect all unique X columns that contain marker pixels
	markerCols := make(map[int]bool)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if isMarker(img, x, y) {
				markerCols[x] = true
			}
		}
	}

	if len(markerCols) == 0 {
		return nil
	}

	// sort columns and split into groups separated by gaps
	cols := make([]int, 0, len(markerCols))
	for c := range markerCols {
		cols = append(cols, c)
	}
	sort.Ints(cols)

	var groups [][]int
	current := []int{cols[0]}
	for i := 1; i < len(cols); i++ {
		if cols[i]-cols[i-1] > 1 {
			groups = append(groups, current)
			current = nil
		}
		current = append(current, cols[i])
	}
	groups = append(groups, current)

	// for each column group, find the vertical extent
	var regions []image.Rectangle
	for _, group := range groups {
		minX, maxX := group[0], group[len(group)-1]
		minY, maxY := bounds.Max.Y, bounds.Min.Y
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for _, x := range group {
				if isMarker(img, x, y) {
					if y < minY {
						minY = y
					}
					if y > maxY {
						maxY = y
					}
					break
				}
			}
		}
		regions = append(regions, image.Rect(minX, minY, maxX+1, maxY+1))
	}

	return regions
}

func clearMarkers(img *image.RGBA, regions []image.Rectangle) {
	for _, region := range regions {
		for y := region.Min.Y; y < region.Max.Y; y++ {
			for x := region.Min.X; x < region.Max.X; x++ {
				if isMarker(img, x, y) {
					img.Set(x, y, color.RGBA{A: 0})
				}
			}
		}
	}
}

func GenerateLicensePlate(ctx Context) {
	selectedRegion := pickRandomRegion()
	plateText := generateFormattedNumber(selectedRegion)
	textColor := parseHexColor(plateConfigs[selectedRegion].Color)

	inputFile, err := os.Open(filepath.Join("assets", "plates", selectedRegion+".png"))
	if err != nil {
		log.Printf("Failed to open plate image for %s: %v", selectedRegion, err)
		return
	}
	defer inputFile.Close()

	img, err := png.Decode(inputFile)
	if err != nil {
		log.Printf("Failed to decode plate image for %s: %v", selectedRegion, err)
		return
	}

	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	regions := findMarkerRegions(rgba)
	if len(regions) == 0 {
		log.Printf("No marker region found in plate image for %s", selectedRegion)
		return
	}
	clearMarkers(rgba, regions)

	fontPath := filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts", "dealerplate california.otf")
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		log.Printf("Failed to read font file: %v", err)
		return
	}

	parsedFont, err := opentype.Parse(fontData)
	if err != nil {
		log.Printf("Failed to parse font: %v", err)
		return
	}

	// split text by spaces to distribute across regions
	segments := strings.Split(plateText, " ")
	if len(segments) < len(regions) {
		// fewer segments than regions — render all text in first region
		segments = []string{plateText}
		regions = regions[:1]
	} else if len(segments) > len(regions) {
		// more segments than regions — join extras into the last region
		joined := strings.Join(segments[len(regions)-1:], " ")
		segments = append(segments[:len(regions)-1], joined)
	}

	for i, region := range regions {
		fontSize := float64(region.Dy())
		face, fErr := opentype.NewFace(parsedFont, &opentype.FaceOptions{
			Size:    fontSize,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if fErr != nil {
			log.Printf("Failed to create font face: %v", fErr)
			return
		}

		metrics := face.Metrics()
		textWidth := font.MeasureString(face, segments[i])
		x := fixed.I(region.Min.X) + (fixed.I(region.Dx())-textWidth)/2
		y := fixed.I(region.Min.Y) + (fixed.I(region.Dy())+metrics.Ascent-metrics.Descent)/2

		drawer := &font.Drawer{
			Dst:  rgba,
			Src:  image.NewUniform(textColor),
			Face: face,
			Dot:  fixed.Point26_6{X: x, Y: y},
		}
		drawer.DrawString(segments[i])
		face.Close()
	}

	ctx.Respond("", rgba)
}
