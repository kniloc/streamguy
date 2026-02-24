package command

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

func PlateRegions() []string {
	keys := make([]string, 0, len(plateConfigs))
	for k := range plateConfigs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func pickRandomRegion() string {
	regions := PlateRegions()
	return regions[rand.Intn(len(regions))]
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

	// for each column group, find the vertical extent then inset to the interior
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

		// measure marker border thickness on each side
		leftInset := 0
		for x := minX; x <= maxX; x++ {
			if !isMarker(img, x, (minY+maxY)/2) {
				break
			}
			leftInset++
		}
		rightInset := 0
		for x := maxX; x >= minX; x-- {
			if !isMarker(img, x, (minY+maxY)/2) {
				break
			}
			rightInset++
		}
		topInset := 0
		for y := minY; y <= maxY; y++ {
			if !isMarker(img, (minX+maxX)/2, y) {
				break
			}
			topInset++
		}
		bottomInset := 0
		for y := maxY; y >= minY; y-- {
			if !isMarker(img, (minX+maxX)/2, y) {
				break
			}
			bottomInset++
		}

		regions = append(regions, image.Rect(
			minX+leftInset, minY+topInset,
			maxX+1-rightInset, maxY+1-bottomInset,
		))
	}

	return regions
}

func clearMarkers(img *image.RGBA) {
	bounds := img.Bounds()

	// collect marker pixel positions
	type pos struct{ x, y int }
	var markers []pos
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if isMarker(img, x, y) {
				markers = append(markers, pos{x, y})
			}
		}
	}

	// replace each marker pixel with the nearest non-marker neighbor color
	for _, p := range markers {
		img.SetRGBA(p.x, p.y, nearestNonMarker(img, p.x, p.y))
	}
}

func nearestNonMarker(img *image.RGBA, px, py int) color.RGBA {
	bounds := img.Bounds()
	for radius := 1; radius <= 10; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				if dx != -radius && dx != radius && dy != -radius && dy != radius {
					continue
				}
				nx, ny := px+dx, py+dy
				if nx >= bounds.Min.X && nx < bounds.Max.X && ny >= bounds.Min.Y && ny < bounds.Max.Y {
					if !isMarker(img, nx, ny) {
						return img.RGBAAt(nx, ny)
					}
				}
			}
		}
	}
	return color.RGBA{A: 255}
}

func GenerateLicensePlate(ctx Context) {
	selectedRegion := strings.ToUpper(strings.TrimSpace(ctx.Args))
	if _, ok := plateConfigs[selectedRegion]; !ok {
		selectedRegion = pickRandomRegion()
	}
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
	clearMarkers(rgba)

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
		segments = []string{plateText}
		regions = regions[:1]
	} else if len(segments) > len(regions) {
		joined := strings.Join(segments[len(regions)-1:], " ")
		segments = append(segments[:len(regions)-1], joined)
	}

	for i, region := range regions {
		// render text at a large size, then stretch to fill the region
		renderSize := math.Max(float64(region.Dy())*2, 200)
		face, fErr := opentype.NewFace(parsedFont, &opentype.FaceOptions{
			Size:    renderSize,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if fErr != nil {
			log.Printf("Failed to create font face: %v", fErr)
			return
		}

		tw := font.MeasureString(face, segments[i])

		var glyphTop, glyphBottom fixed.Int26_6
		for _, ch := range segments[i] {
			b, _, ok := face.GlyphBounds(ch)
			if !ok {
				continue
			}
			if b.Min.Y < glyphTop {
				glyphTop = b.Min.Y
			}
			if b.Max.Y > glyphBottom {
				glyphBottom = b.Max.Y
			}
		}
		glyphH := (glyphBottom - glyphTop).Ceil()
		textW := tw.Ceil()

		// draw text onto a tight temporary image
		tmp := image.NewRGBA(image.Rect(0, 0, textW, glyphH))
		drawer := &font.Drawer{
			Dst:  tmp,
			Src:  image.NewUniform(textColor),
			Face: face,
			Dot:  fixed.Point26_6{X: 0, Y: -glyphTop},
		}
		drawer.DrawString(segments[i])
		face.Close()

		// scale the temporary image to fill the entire marker region
		xdraw.BiLinear.Scale(rgba, region, tmp, tmp.Bounds(), xdraw.Over, nil)
	}

	ctx.Respond("", rgba)
}
