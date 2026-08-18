package music

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

const DefaultBPM = 120
const DefaultDynamic = DynF

type Parser struct {
	tokens []Token
	pos    int
}

func NewParser(tokens []Token) *Parser {
	return &Parser{tokens: tokens}
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *Parser) advance() Token {
	t := p.tokens[p.pos]
	p.pos++
	return t
}

func (p *Parser) expect(kind TokenKind) (Token, error) {
	t := p.peek()
	if t.Kind != kind {
		return Token{}, fmt.Errorf("pos %d: expected token kind %d, got %d (%q)", t.Pos, kind, t.Kind, t.Value)
	}
	return p.advance(), nil
}

func (p *Parser) Parse() (*Score, error) {
	score := &Score{}
	for {
		track, err := p.parseTrack()
		if err != nil {
			return nil, err
		}
		score.Tracks = append(score.Tracks, *track)

		if p.peek().Kind != TokenBar {
			break
		}
		p.advance()
	}

	if p.peek().Kind != TokenEOF {
		t := p.peek()
		return nil, fmt.Errorf("pos %d: unexpected token %q after score end", t.Pos, t.Value)
	}

	return score, nil
}

func (p *Parser) parseTrack() (*Track, error) {
	track := &Track{BPM: DefaultBPM}
	currentDynamic := DefaultDynamic

	if p.peek().Kind == TokenTempo {
		bpm, err := parseTempo(p.advance())
		if err != nil {
			return nil, err
		}
		track.BPM = bpm
	}

	for {
		switch p.peek().Kind {
		case TokenEOF, TokenBar:
			return track, nil

		case TokenTempo:
			bpm, err := parseTempo(p.advance())
			if err != nil {
				return nil, err
			}
			track.BPM = bpm

		case TokenDynamic:
			dyn, err := parseDynamic(p.advance())
			if err != nil {
				return nil, err
			}
			currentDynamic = dyn

		case TokenNote:
			event, err := parseNote(p.advance(), currentDynamic)
			if err != nil {
				return nil, err
			}
			track.Events = append(track.Events, event)

		case TokenRest:
			event, err := parseRest(p.advance())
			if err != nil {
				return nil, err
			}
			track.Events = append(track.Events, event)

		case TokenChordOpen:
			event, err := p.parseChord(currentDynamic)
			if err != nil {
				return nil, err
			}
			track.Events = append(track.Events, event)

		default:
			t := p.peek()
			return nil, fmt.Errorf("pos %d: unexpected token %q in track", t.Pos, t.Value)
		}
	}
}

func (p *Parser) parseChord(dyn Dynamic) (Event, error) {
	_, err := p.expect(TokenChordOpen)
	if err != nil {
		return Event{}, err
	}

	var pitches []Pitch
	for p.peek().Kind == TokenPitch {
		pitch, pErr := parsePitch(p.advance().Value)
		if pErr != nil {
			return Event{}, pErr
		}
		pitches = append(pitches, pitch)
	}

	if len(pitches) == 0 {
		return Event{}, fmt.Errorf("chord must contain at least one pitch")
	}

	_, err = p.expect(TokenChordClose)
	if err != nil {
		return Event{}, err
	}

	duration, err := p.expect(TokenDuration)
	if err != nil {
		return Event{}, fmt.Errorf("expected duration after chord: %w", err)
	}
	dur, err := parseDuration(duration.Value)
	if err != nil {
		return Event{}, err
	}

	return Event{
		Kind:     EventChord,
		Pitches:  pitches,
		Duration: dur,
		Dynamic:  dyn,
	}, nil
}

// *** Token value parsers ***

func parseTempo(t Token) (int, error) {
	// e.g. "t120"
	s := strings.TrimPrefix(t.Value, "t")
	bpm, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("pos %d: invalid tempo %q", t.Pos, t.Value)
	}
	if bpm < 30 || bpm > 250 {
		return 0, fmt.Errorf("pos %d: tempo %d out of range (30-250)", t.Pos, bpm)
	}
	return bpm, nil
}

func parseDynamic(t Token) (Dynamic, error) {
	switch t.Value {
	case "pp":
		return DynPP, nil
	case "p":
		return DynP, nil
	case "mp":
		return DynMP, nil
	case "mf":
		return DynMF, nil
	case "f":
		return DynF, nil
	case "ff":
		return DynFF, nil
	}
	return 0, fmt.Errorf("pos %d: unknown dynamic %q", t.Pos, t.Value)
}

func parseNote(t Token, dyn Dynamic) (Event, error) {
	// e.g. "C4q", "Bb3w"
	v := t.Value
	pitch, rest, err := extractPitch(v, t.Pos)
	if err != nil {
		return Event{}, err
	}
	dur, err := parseDuration(rest)
	if err != nil {
		return Event{}, fmt.Errorf("pos %d: %w", t.Pos, err)
	}
	return Event{
		Kind:     EventNote,
		Pitches:  []Pitch{pitch},
		Duration: dur,
		Dynamic:  dyn,
	}, nil
}

func parseRest(t Token) (Event, error) {
	// e.g. "rq", "rh"
	dur, err := parseDuration(strings.TrimPrefix(t.Value, "r"))
	if err != nil {
		return Event{}, fmt.Errorf("pos %d: %w", t.Pos, err)
	}
	return Event{Kind: EventRest, Duration: dur}, nil
}

func parsePitch(s string) (Pitch, error) {
	pitch, rest, err := extractPitch(s, 0)
	if err != nil {
		return Pitch{}, err
	}
	if rest != "" {
		return Pitch{}, fmt.Errorf("unexpected trailing %q in pitch", rest)
	}
	return pitch, nil
}

func extractPitch(s string, pos int) (Pitch, string, error) {
	if len(s) == 0 {
		return Pitch{}, "", fmt.Errorf("pos %d: empty pitch", pos)
	}

	name := string(s[0])
	if name < "A" || name > "G" {
		return Pitch{}, "", fmt.Errorf("pos %d: invalid note name %q", pos, name)
	}
	s = s[1:]

	accidental := 0
	if len(s) > 0 && (s[0] == '#' || s[0] == 'b') {
		if s[0] == '#' {
			accidental = 1
		} else {
			accidental = -1
		}
		s = s[1:]
	}

	if len(s) == 0 || !unicode.IsDigit(rune(s[0])) {
		return Pitch{}, "", fmt.Errorf("pos %d: expected octave digit after %s", pos, name)
	}
	octave := int(s[0] - '0')
	s = s[1:]

	return Pitch{Name: name, Accidental: accidental, Octave: octave}, s, nil
}

func parseDuration(s string) (Duration, error) {
	if len(s) == 0 {
		return Duration{}, fmt.Errorf("empty duration")
	}
	var base BaseDuration
	switch s[0] {
	case 'w':
		base = Whole
	case 'h':
		base = Half
	case 'q':
		base = Quarter
	case 'e':
		base = Eighth
	case 's':
		base = Sixteenth
	default:
		return Duration{}, fmt.Errorf("unknown duration %q (expected w/h/q/e/s)", s[0])
	}
	dotted := len(s) > 1 && s[1] == '.'
	if len(s) > 1 && !dotted {
		return Duration{}, fmt.Errorf("unexpected character %q after duration", s[1])
	}
	return Duration{Base: base, Dotted: dotted}, nil
}

// Parse is the top-level entry point
func Parse(input string) (*Score, error) {
	lexer := NewLexer(input)
	tokens, err := lexer.Tokenize()
	if err != nil {
		return nil, fmt.Errorf("lex error: %w", err)
	}
	parser := NewParser(tokens)
	score, err := parser.Parse()
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	return score, nil
}
