package render

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"stream-guy/internal/assets"
)

type Emote struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Name       string `json:"name"`
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
	ImageURL   string `json:"imageUrl"`
}

type TextParser struct{}

func NewTextParser() *TextParser {
	return &TextParser{}
}

func (tp *TextParser) Parse(message string, twitchEmotes []Emote) []assets.EmoteSegment {
	validEmotes := tp.filterValidEmotes(twitchEmotes)

	if len(validEmotes) == 0 {
		segments := tp.parseUnicodeEmojis(message)
		if len(segments) == 0 {
			return []assets.EmoteSegment{{IsEmote: false, Text: message}}
		}
		return segments
	}

	return tp.mergeEmotesAndEmojis(message, validEmotes)
}

func (tp *TextParser) filterValidEmotes(emotes []Emote) []Emote {
	var valid []Emote
	for _, e := range emotes {
		if e.Type == "Twemoji" {
			continue
		}

		if tp.isValidEmoteURL(e.ImageURL) {
			valid = append(valid, e)
		}
	}
	return valid
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

func (tp *TextParser) mergeEmotesAndEmojis(message string, twitchEmotes []Emote) []assets.EmoteSegment {
	var segments []assets.EmoteSegment
	bytePos := 0
	charPos := 0

	emoteMap := make(map[int]Emote)
	for _, e := range twitchEmotes {
		emoteMap[e.StartIndex] = e
	}

	for bytePos < len(message) {
		if emote, exists := emoteMap[charPos]; exists {
			segments = append(segments, assets.EmoteSegment{
				IsEmote:  true,
				Text:     emote.Name,
				ImageURL: emote.ImageURL,
			})

			emoteLen := emote.EndIndex - emote.StartIndex + 1
			for range emoteLen {
				_, size := utf8.DecodeRuneInString(message[bytePos:])
				bytePos += size
				charPos++
			}
			continue
		}

		r, size := utf8.DecodeRuneInString(message[bytePos:])
		if tp.isEmoji(r) {
			var emojiSequence []rune
			for bytePos < len(message) {
				r, size = utf8.DecodeRuneInString(message[bytePos:])
				if !tp.isEmoji(r) {
					break
				}
				emojiSequence = append(emojiSequence, r)
				bytePos += size
				charPos++
			}
			segments = append(segments, tp.createEmojiSegment(emojiSequence))
			continue
		}

		var textBuilder strings.Builder
		for bytePos < len(message) {
			if _, exists := emoteMap[charPos]; exists {
				break
			}

			r, size = utf8.DecodeRuneInString(message[bytePos:])
			if tp.isEmoji(r) {
				break
			}

			textBuilder.WriteRune(r)
			bytePos += size
			charPos++
		}

		if textBuilder.Len() > 0 {
			segments = append(segments, assets.EmoteSegment{
				IsEmote: false,
				Text:    textBuilder.String(),
			})
		}
	}

	return segments
}
