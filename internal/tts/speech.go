package tts

import (
	"errors"
	"sync"

	"github.com/Edw590/sapi-go"
	"github.com/go-ole/go-ole"
)

var (
	once      sync.Once
	initError error
	instance  *Speech
)

const (
	SFalse = 0x00000001
)

type Speech struct {
	sapi *sapi.Sapi
	mu   sync.Mutex
}

func InitSpeech() error {
	once.Do(func() {
		if err := ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED); err != nil {
			var oleError *ole.OleError
			ok := errors.As(err, &oleError)

			if !ok || oleError.Code() != SFalse {
				initError = err
				return
			}
		}

		sappy, err := sapi.NewSapi()
		if err != nil {
			initError = err
			return
		}

		instance = &Speech{sapi: sappy}
	})
	return initError
}

func GetSpeech() *Speech {
	return instance
}

func (sp *Speech) Speak(text string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	_, err := sp.sapi.Speak(text, sapi.SVSFlagsAsync|sapi.SVSFPurgeBeforeSpeak)
	return err
}

func (sp *Speech) SpeakSync(text string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	_, err := sp.sapi.Speak(text, sapi.SVSFDefault)
	return err
}

func (sp *Speech) SpeakQueued(text string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	_, err := sp.sapi.Speak(text, sapi.SVSFlagsAsync)
	return err
}

func (sp *Speech) SetRate(rate int) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	return sp.sapi.SetRate(rate)
}

func (sp *Speech) SetVolume(volume int) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	return sp.sapi.SetVolume(volume)
}

func (sp *Speech) Stop() error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	_, err := sp.sapi.Speak("", sapi.SVSFPurgeBeforeSpeak)
	return err
}

func (sp *Speech) Wait(timeout int) (bool, error) {
	return sp.sapi.WaitUntilDone(timeout)
}
