package media

import (
	"fmt"
	"math"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/domain"
)

const (
	CalibrationSampleRate = 48000
	CalibrationFrames     = 24
	CalibrationAudioMS    = 1000
	CalibrationAudioEvent = 500
	CalibrationVideoEvent = 9
	CalibrationFPS        = 24
)

func CalibrationPair() ([]byte, []byte, error) {
	sampleCount := CalibrationSampleRate * CalibrationAudioMS / 1000
	samples := make([]int16, sampleCount)
	eventSample := CalibrationSampleRate * CalibrationAudioEvent / 1000
	for index := eventSample; index < sampleCount; index++ {
		elapsed := float64(index-eventSample) / CalibrationSampleRate
		envelope := math.Exp(-elapsed * 7)
		value := math.Sin(2*math.Pi*440*float64(index-eventSample)/CalibrationSampleRate) * 12000 * envelope
		samples[index] = int16(math.Round(value))
	}
	audio, err := EncodePCM16(CalibrationSampleRate, 1, samples)
	if err != nil {
		return nil, nil, err
	}

	width, height := 96, 54
	frameBytes := width*height + 2*(width/2)*(height/2)
	frames := make([][]byte, 0, CalibrationFrames)
	for frameIndex := 0; frameIndex < CalibrationFrames; frameIndex++ {
		frame := make([]byte, frameBytes)
		luma := byte(32)
		if frameIndex == CalibrationVideoEvent {
			luma = 220
		}
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				frame[y*width+x] = luma
			}
		}
		chromaSize := (width / 2) * (height / 2)
		for index := 0; index < chromaSize; index++ {
			frame[width*height+index] = 128
			frame[width*height+chromaSize+index] = 128
		}
		frames = append(frames, frame)
	}
	info := domain.VideoInfo{
		Width:      width,
		Height:     height,
		FPSNum:     CalibrationFPS,
		FPSDen:     1,
		FrameCount: len(frames),
		Chroma:     "C420jpeg",
	}
	video, err := EncodeY4M(info, frames)
	if err != nil {
		return nil, nil, err
	}
	return audio, video, nil
}

func MustCalibrationPair() ([]byte, []byte) {
	audio, video, err := CalibrationPair()
	if err != nil {
		panic(fmt.Sprintf("calibration media construction failed: %v", err))
	}
	return audio, video
}
