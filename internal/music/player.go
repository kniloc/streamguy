package music

/*
#include <fluidsynth.h>
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

const (
	DefaultInstrument = 0 // acoustic grand piano
)

// Player holds the FluidSynth settings, synth, and audio driver.
// Create one with NewPlayer, play scores with Play, and close when done.
type Player struct {
	settings *C.fluid_settings_t
	synth    *C.fluid_synth_t
	driver   *C.fluid_audio_driver_t
	sfontID  C.int
}

func NewPlayer(sfPath string) (*Player, error) {
	settings := C.new_fluid_settings()
	if settings == nil {
		return nil, fmt.Errorf("failed to create FluidSynth settings")
	}

	synth := C.new_fluid_synth(settings)
	if synth == nil {
		C.delete_fluid_settings(settings)
		return nil, fmt.Errorf("failed to create FluidSynth synth")
	}

	driver := C.new_fluid_audio_driver(settings, synth)
	if driver == nil {
		C.delete_fluid_synth(synth)
		C.delete_fluid_settings(settings)
		return nil, fmt.Errorf("failed to create audio driver")
	}

	// Load soundfont
	path := C.CString(sfPath)
	defer C.free(unsafe.Pointer(path))

	sfontID := C.fluid_synth_sfload(synth, path, C.int(1))
	if sfontID == C.FLUID_FAILED {
		C.delete_fluid_audio_driver(driver)
		C.delete_fluid_synth(synth)
		C.delete_fluid_settings(settings)
		return nil, fmt.Errorf("failed to load soundfont at %s", sfPath)
	}

	return &Player{
		settings: settings,
		synth:    synth,
		driver:   driver,
		sfontID:  sfontID,
	}, nil
}

func (p *Player) Close() {
	if p.driver != nil {
		C.delete_fluid_audio_driver(p.driver)
	}
	if p.synth != nil {
		C.delete_fluid_synth(p.synth)
	}
	if p.settings != nil {
		C.delete_fluid_settings(p.settings)
	}
}

func (p *Player) Play(score *Score) error {
	if len(score.Tracks) == 0 {
		return nil
	}
	if len(score.Tracks) > 16 {
		return fmt.Errorf("score has %d tracks; MIDI supports a maximum of 16", len(score.Tracks))
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(score.Tracks))

	for i, track := range score.Tracks {
		wg.Add(1)
		go func(channel int, track Track) {
			defer wg.Done()
			if err := p.playTrack(channel, track); err != nil {
				errs <- fmt.Errorf("track %d: %w", channel, err)
			}
		}(i, track)
	}

	wg.Wait()
	close(errs)

	// Return first error (if any)
	for err := range errs {
		return err
	}
	return nil
}

func (p *Player) playTrack(channel int, track Track) error {
	ch := C.int(channel)
	secPerBeat := 60.0 / float64(track.BPM)

	// Set instrument
	C.fluid_synth_program_change(p.synth, ch, C.int(DefaultInstrument))

	for _, event := range track.Events {
		dur := time.Duration(event.Duration.Beats()*secPerBeat*1000) * time.Millisecond
		velocity := dynToVelocity(event.Dynamic)

		switch event.Kind {
		case EventRest:
			time.Sleep(dur)

		case EventNote:
			midi := C.int(event.Pitches[0].MIDINote())
			C.fluid_synth_noteon(p.synth, ch, midi, C.int(velocity))
			time.Sleep(dur)
			C.fluid_synth_noteoff(p.synth, ch, midi)

		case EventChord:
			for _, pitch := range event.Pitches {
				midi := C.int(pitch.MIDINote())
				C.fluid_synth_noteon(p.synth, ch, midi, C.int(velocity))
			}
			time.Sleep(dur)
			for _, pitch := range event.Pitches {
				midi := C.int(pitch.MIDINote())
				C.fluid_synth_noteoff(p.synth, ch, midi)
			}
		}
	}
	return nil
}

func dynToVelocity(d Dynamic) int {
	switch d {
	case DynPP:
		return 20
	case DynP:
		return 40
	case DynMP:
		return 60
	case DynMF:
		return 80
	case DynF:
		return 100
	case DynFF:
		return 120
	}
	return 80 // default to mf
}
