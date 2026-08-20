package render

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"stream-guy/internal/assets"
)

type MessagePart struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	ImageURL  string `json:"imageURL"`
	Source    string `json:"source"`
	ZeroWidth bool   `json:"zeroWidth"`
}

type TextParser struct{}

func NewTextParser() *TextParser {
	return &TextParser{}
}

func (tp *TextParser) ParseFromParts(parts []MessagePart) []assets.EmoteSegment {
	if len(parts) == 0 {
		return []assets.EmoteSegment{}
	}

	var segments []assets.EmoteSegment
	for _, part := range parts {
		switch part.Type {
		case "emote":
			if !tp.isValidEmoteURL(part.ImageURL) {
				segments = append(segments, assets.EmoteSegment{IsEmote: false, Text: part.Text})
				continue
			}
			segments = append(segments, assets.EmoteSegment{
				IsEmote:  true,
				Text:     part.Text,
				ImageURL: part.ImageURL,
			})
		case "text":
			emojiSegs := tp.parseUnicodeEmojis(part.Text)
			if len(emojiSegs) > 0 {
				segments = append(segments, emojiSegs...)
			} else {
				segments = append(segments, assets.EmoteSegment{IsEmote: false, Text: part.Text})
			}
		default:
			if part.Text != "" {
				segments = append(segments, assets.EmoteSegment{IsEmote: false, Text: part.Text})
			}
		}
	}
	return segments
}

func (tp *TextParser) isValidEmoteURL(url string) bool {
	if url == "" {
		return false
	}

	if strings.HasSuffix(url, "/.png") ||
		strings.HasSuffix(url, "/.jpg") ||
		strings.HasSuffix(url, "/.gif") ||
		strings.HasSuffix(url, "/.webp") {
		return false
	}

	return true
}

func (tp *TextParser) isEmoji(r rune) bool {
	return (r >= 0x1F600 && r <= 0x1F64F) ||
		(r >= 0x1F300 && r <= 0x1F5FF) ||
		(r >= 0x1F680 && r <= 0x1F6FF) ||
		(r >= 0x1F1E0 && r <= 0x1F1FF) ||
		(r >= 0x2600 && r <= 0x26FF) ||
		(r >= 0x2700 && r <= 0x27BF) ||
		(r >= 0xFE00 && r <= 0xFE0F) ||
		(r >= 0x1F900 && r <= 0x1F9FF) ||
		(r >= 0x1FA00 && r <= 0x1FA6F) ||
		(r >= 0x1FA70 && r <= 0x1FAFF) ||
		r == 0x200D ||
		r == 0x20E3
}

func (tp *TextParser) parseUnicodeEmojis(text string) []assets.EmoteSegment {
	var segments []assets.EmoteSegment
	var currentText strings.Builder
	var emojiSequence []rune

	i := 0
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])

		if tp.isEmoji(r) {
			if currentText.Len() > 0 {
				segments = append(segments, assets.EmoteSegment{
					IsEmote: false,
					Text:    currentText.String(),
				})
				currentText.Reset()
			}

			emojiSequence = append(emojiSequence, r)
			i += size

			if i < len(text) {
				nextR, _ := utf8.DecodeRuneInString(text[i:])
				if !tp.isEmoji(nextR) {
					segments = append(segments, tp.createEmojiSegment(emojiSequence))
					emojiSequence = nil
				}
			} else {
				segments = append(segments, tp.createEmojiSegment(emojiSequence))
				emojiSequence = nil
			}
		} else {
			if len(emojiSequence) > 0 {
				segments = append(segments, tp.createEmojiSegment(emojiSequence))
				emojiSequence = nil
			}

			currentText.WriteRune(r)
			i += size
		}
	}

	if currentText.Len() > 0 {
		segments = append(segments, assets.EmoteSegment{
			IsEmote: false,
			Text:    currentText.String(),
		})
	}

	if len(emojiSequence) > 0 {
		segments = append(segments, tp.createEmojiSegment(emojiSequence))
	}

	return segments
}

func (tp *TextParser) createEmojiSegment(runes []rune) assets.EmoteSegment {
	emojiText := string(runes)

	var codepoints []string
	for _, r := range runes {
		if r >= 0xFE00 && r <= 0xFE0F {
			continue
		}
		codepoints = append(codepoints, fmt.Sprintf("%x", r))
	}

	filename := strings.Join(codepoints, "-")
	url := fmt.Sprintf("https://%s/assets/72x72/%s.png", assets.TwemojiCDNPath, filename)

	return assets.EmoteSegment{
		IsEmote:  true,
		Text:     emojiText,
		ImageURL: url,
	}
}
