package assets

import (
	"os"
	"path/filepath"

	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/widget/material"
)

const FontName = "AppleGaramond-Bold.ttf"

func LoadFont(fontName string) (font.FontFace, error) {
	fontPath := filepath.Join(os.Getenv("WINDIR"), "Fonts", fontName)

	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		return font.FontFace{}, err
	}

	face, err := opentype.Parse(fontData)
	if err != nil {
		return font.FontFace{}, err
	}

	return font.FontFace{
		Font: font.Font{Typeface: font.Typeface(fontName)},
		Face: face,
	}, nil
}

func WithFont(l material.LabelStyle, typeface string) material.LabelStyle {
	l.Font.Typeface = font.Typeface(typeface)
	return l
}
