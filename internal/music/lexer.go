package music

import (
	"fmt"
	"strings"
	"unicode"
)

type TokenKind int

const (
	TokenTempo TokenKind = iota
	TokenNote
	TokenRest
	TokenChordOpen
	TokenChordClose
	TokenPitch
	TokenDuration
	TokenDynamic
	TokenBar
	TokenEOF
)

type Token struct {
	Kind  TokenKind
	Value string
	Pos   int
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%d, %q, pos=%d)", t.Kind, t.Value, t.Pos)
}

type Lexer struct {
	input []rune
	pos   int
}

func NewLexer(input string) *Lexer {
	return &Lexer{input: []rune(strings.TrimSpace(input))}
}

func (l *Lexer) peek() (rune, bool) {
	if l.pos >= len(l.input) {
		return 0, false
	}
	return l.input[l.pos], true
}

func (l *Lexer) peekAt(offset int) (rune, bool) {
	i := l.pos + offset
	if l.pos >= len(l.input) {
		return 0, false
	}
	return l.input[i], true
}

func (l *Lexer) advance() rune {
	r := l.input[l.pos]
	l.pos++
	return r
}

func (l *Lexer) skipWhitespace() {
	for {
		r, ok := l.peek()
		if !ok || !unicode.IsSpace(r) {
			break
		}
		l.advance()
	}
}

func isNoteChar(r rune) bool {
	return r >= 'A' && r <= 'G'
}

func isDurationChar(r rune) bool {
	return r == 'w' || r == 'h' || r == 'q' || r == 'e' || r == 's'
}

func (l *Lexer) Tokenize() ([]Token, error) {
	var tokens []Token

	for {
		l.skipWhitespace()
		start := l.pos
		r, ok := l.peek()

		if !ok {
			tokens = append(tokens, Token{Kind: TokenEOF, Pos: l.pos})
			break
		}

		switch {
		// Bar separator
		case r == '|':
			l.advance()
			tokens = append(tokens, Token{Kind: TokenBar, Value: "|", Pos: start})

		// Chord open
		case r == '[':
			l.advance()
			tokens = append(tokens, Token{Kind: TokenChordOpen, Value: "[", Pos: start})

		// Chord open
		case r == ']':
			l.advance()
			tokens = append(tokens, Token{Kind: TokenChordClose, Value: "]", Pos: start})

		// Tempo: t followed by digits
		case r == 't':
			tempo, err := l.readTempo(start)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, tempo)

		// Rest: r followed by duration
		case r == 'r':
			rest, err := l.readRest(start)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, rest)

		// Dynamics
		case r == 'p' || r == 'f' || r == 'm':
			tkn, isDyn, err := l.readDynamic(start)
			if err != nil {
				return nil, err
			}
			if isDyn {
				tokens = append(tokens, tkn)
			} else {
				return nil, fmt.Errorf("pos %d: unexpected character %q", start, r)
			}

		// Duration after chord close
		// Only reached if we're in a chord-close context; handled as TokenDuration
		case isDurationChar(r):
			dur, err := l.readDuration(start)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, dur)

		// Notes
		case isNoteChar(r):
			note, err := l.readNoteOrPitch(start)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, note)

		default:
			return nil, fmt.Errorf("pos %d: unexpected character %q", start, r)
		}
	}

	return tokens, nil
}

func (l *Lexer) readTempo(start int) (Token, error) {
	l.advance()
	var s strings.Builder
	s.WriteString("t")
	for {
		r, ok := l.peek()
		if !ok || !unicode.IsDigit(r) {
			break
		}
		s.WriteString(string(l.advance()))
	}
	if s.String() == "t" {
		return Token{}, fmt.Errorf("pos %d: expected digits after 't", start)
	}
	return Token{Kind: TokenTempo, Value: s.String(), Pos: start}, nil
}

func (l *Lexer) readRest(start int) (Token, error) {
	l.advance()
	dur, err := l.readDurationString(start)
	if err != nil {
		return Token{}, err
	}
	return Token{Kind: TokenRest, Value: "r" + dur, Pos: start}, nil
}

func (l *Lexer) readDurationString(start int) (string, error) {
	r, ok := l.peek()
	if !ok || !isDurationChar(r) {
		return "", fmt.Errorf("pos %d: expected duration (w/h/q/e/s)", start)
	}
	s := string(l.advance())
	if dot, hasDot := l.peek(); hasDot && dot == '.' {
		s += string(l.advance())
	}
	return s, nil
}

func (l *Lexer) readDuration(start int) (Token, error) {
	dur, err := l.readDurationString(start)
	if err != nil {
		return Token{}, err
	}
	return Token{Kind: TokenDuration, Value: dur, Pos: start}, nil
}

func (l *Lexer) readDynamic(start int) (Token, bool, error) {
	r, _ := l.peek()
	next, hasNext := l.peekAt(1)

	// Check for two-character dynamics first
	if hasNext {
		two := string(r) + string(next)
		switch two {
		case "pp", "mp", "mf", "ff":
			after, _ := l.peekAt(2)
			if !isNoteChar(after) && !unicode.IsDigit(after) && after != '#' && after != 'b' {
				l.advance()
				l.advance()
				return Token{Kind: TokenDynamic, Value: two, Pos: start}, true, nil
			}
		}
	}

	switch r {
	case 'p':
		after, _ := l.peekAt(1)
		if !isNoteChar(after) && !unicode.IsDigit(after) && after != '#' {
			l.advance()
			return Token{Kind: TokenDynamic, Value: "p", Pos: start}, true, nil
		}
	case 'f':
		after, _ := l.peekAt(1)
		if !isNoteChar(after) && !unicode.IsDigit(after) && after != '#' {
			l.advance()
			return Token{Kind: TokenDynamic, Value: "f", Pos: start}, true, nil
		}
	}

	return Token{}, false, nil
}

func (l *Lexer) readNoteOrPitch(start int) (Token, error) {
	name := string(l.advance())

	accidental := ""
	if r, acc := l.peek(); acc && (r == '#' || r == 'b') {
		accidental = string(l.advance())
	}

	if r, oct := l.peek(); !oct || !unicode.IsDigit(r) {
		return Token{}, fmt.Errorf("pos %d: expected octave digit after note %s%s", start, name, accidental)
	}
	octave := string(l.advance())

	if r, ok := l.peek(); ok && isDurationChar(r) {
		dur := string(l.advance())
		if dot, hasDot := l.peek(); hasDot && dot == '.' {
			dur += string(l.advance())
		}
		return Token{Kind: TokenNote, Value: name + accidental + octave + dur, Pos: start}, nil
	}

	return Token{Kind: TokenPitch, Value: name + accidental + octave, Pos: start}, nil
}
