package music

import (
	"fmt"
	"math"
)

type BaseDuration int

type Duration struct {
	Base   BaseDuration
	Dotted bool
}

const (
	Whole BaseDuration = iota
	Half
	Quarter
	Eighth
	Sixteenth
)

func (b BaseDuration) String() string {
	switch b {
	case Whole:
		return "w"
	case Half:
		return "h"
	case Quarter:
		return "q"
	case Eighth:
		return "e"
	case Sixteenth:
		return "s"
	}
	return "?"
}

func (d Duration) String() string {
	s := d.Base.String()
	if d.Dotted {
		s += "."
	}
	return s
}

// Beats returns the duration length (quarter note = 1.0)
func (d Duration) Beats() float64 {
	var base float64

	switch d.Base {
	case Whole:
		base = 4.0
	case Half:
		base = 2.0
	case Quarter:
		base = 1.0
	case Eighth:
		base = 0.5
	case Sixteenth:
		base = 0.25
	}

	if d.Dotted {
		base *= 1.5
	}
	return base
}

type Dynamic int

const (
	DynPP Dynamic = iota
	DynP
	DynMP
	DynMF
	DynF
	DynFF
)

func (d Dynamic) String() string {
	switch d {
	case DynPP:
		return "pp"
	case DynP:
		return "p"
	case DynMP:
		return "mp"
	case DynMF:
		return "mf"
	case DynF:
		return "f"
	case DynFF:
		return "ff"
	}
	return "?"
}

// Gain returns a linear gain value (0.0 - 1.0)
func (d Dynamic) Gain() float64 {
	switch d {
	case DynPP:
		return 0.1
	case DynP:
		return 0.25
	case DynMP:
		return 0.45
	case DynMF:
		return 0.65
	case DynF:
		return 0.8
	case DynFF:
		return 1.0
	}
	return 0.65
}

type Pitch struct {
	Name       string
	Accidental int
	Octave     int
}

func (p Pitch) String() string {
	s := p.Name
	switch p.Accidental {
	case 1:
		s += "#"
	case -1:
		s += "b"
	}
	s += fmt.Sprintf("%d", p.Octave)
	return s
}

// MIDINote returns the MIDI note number (C4 = 60)
func (p Pitch) MIDINote() int {
	semitones := map[string]int{
		"C": 0, "D": 2, "E": 4, "F": 5,
		"G": 7, "A": 9, "B": 11,
	}
	base := semitones[p.Name]
	return 12*(p.Octave+1) + base + p.Accidental
}

func (p Pitch) Frequency() float64 {
	midi := p.MIDINote()
	return 440.0 * math.Pow(2, (float64(midi)-69.0)/12.0)
}

type EventKind int

const (
	EventNote EventKind = iota
	EventChord
	EventRest
)

type Event struct {
	Kind     EventKind
	Pitches  []Pitch
	Duration Duration
	Dynamic  Dynamic
}

func (e Event) String() string {
	switch e.Kind {
	case EventRest:
		return fmt.Sprintf("rest(%s)", e.Duration)
	case EventNote:
		return fmt.Sprintf("note(%s %s dyn=%s)", e.Pitches[0], e.Duration, e.Dynamic)
	case EventChord:
		s := "chord("
		for i, pitch := range e.Pitches {
			if i > 0 {
				s += "+"
			}
			s += pitch.String()
		}
		return s + fmt.Sprintf(" %s dyn=%s)", e.Duration, e.Dynamic)
	}
	return "?"
}

type Track struct {
	BPM    int
	Events []Event
}

func (t Track) DurationInSeconds() float64 {
	total := 0.0
	secPerBeat := 60.0 / float64(t.BPM)
	for _, event := range t.Events {
		total += event.Duration.Beats() * secPerBeat
	}
	return total
}

// Score is a full-parsed composition
type Score struct {
	Tracks []Track
}
