package media

import (
	"fmt"
)

// PictureTiming describes the measurable timeline of a picture stream. It
// exposes flash times near the clapper (first flash) and near the end of the
// take (last flash) together with the overall duration, so a frame-rate
// conversion can be checked for preserved playback timing at both ends.
type PictureTiming struct {
	FPSNum          int
	FPSDen          int
	FrameCount      int
	DurationMS      int
	FirstFlashMS    int
	LastFlashMS     int
	FirstFlashFrame int
	LastFlashFrame  int
}

// MeasurePictureTiming probes a Y4M payload and locates its first and last
// luma flashes without modifying the bytes.
func MeasurePictureTiming(data []byte) (PictureTiming, error) {
	clip, err := InspectVideo(data)
	if err != nil {
		return PictureTiming{}, fmt.Errorf("picture probe failed: %w", err)
	}
	first, last, err := locatePictureFlashes(clip)
	if err != nil {
		return PictureTiming{}, err
	}
	frameMS := func(index int) int {
		return index * 1000 * clip.Info.FPSDen / clip.Info.FPSNum
	}
	return PictureTiming{
		FPSNum:          clip.Info.FPSNum,
		FPSDen:          clip.Info.FPSDen,
		FrameCount:      clip.Info.FrameCount,
		DurationMS:      clip.Info.DurationMS,
		FirstFlashMS:    frameMS(first),
		LastFlashMS:     frameMS(last),
		FirstFlashFrame: first,
		LastFlashFrame:  last,
	}, nil
}

// FramePeriodMS returns the display duration of one frame rounded to the
// nearest millisecond with a floor of one millisecond. It is the natural
// tolerance when comparing event times across two frame rates.
func (t PictureTiming) FramePeriodMS() int {
	period := t.FPSDen * 1000 / t.FPSNum
	if period < 1 {
		return 1
	}
	return period
}

// VerifyPreservedTiming asserts that a converted picture keeps the source
// playback timeline: the clapper flash, the tail flash, and the overall
// duration must each stay within one output frame period. It rejects the
// label-only failure mode where the frame count is left untouched while the
// rate changes, which would fast- or slow-play the material.
func VerifyPreservedTiming(source, output PictureTiming) error {
	tolerance := output.FramePeriodMS()
	if diff := absInt(source.FirstFlashMS - output.FirstFlashMS); diff > tolerance {
		return fmt.Errorf(
			"clapper flash moved by %d ms, beyond the %d ms frame period",
			diff,
			tolerance,
		)
	}
	if diff := absInt(source.LastFlashMS - output.LastFlashMS); diff > tolerance {
		return fmt.Errorf(
			"tail flash moved by %d ms, beyond the %d ms frame period",
			diff,
			tolerance,
		)
	}
	if diff := absInt(source.DurationMS - output.DurationMS); diff > tolerance {
		return fmt.Errorf(
			"playback duration changed by %d ms, beyond the %d ms frame period",
			diff,
			tolerance,
		)
	}
	if output.FPSNum == source.FPSNum && output.FPSDen == source.FPSDen &&
		output.FrameCount != source.FrameCount {
		return fmt.Errorf("frame count changed without a frame-rate conversion")
	}
	return nil
}

// AudioTiming captures the dimensions that prove audio speed was not altered.
type AudioTiming struct {
	SampleRate int
	Channels   int
	FrameCount int
	DurationMS int
}

// MeasureAudioTiming probes a WAV payload for its sample-accurate dimensions.
func MeasureAudioTiming(data []byte) (AudioTiming, error) {
	clip, err := InspectAudio(data)
	if err != nil {
		return AudioTiming{}, fmt.Errorf("audio probe failed: %w", err)
	}
	return AudioTiming{
		SampleRate: clip.Info.SampleRate,
		Channels:   clip.Info.Channels,
		FrameCount: clip.Info.FrameCount,
		DurationMS: clip.Info.DurationMS,
	}, nil
}

// VerifyUnchangedAudio asserts the conversion neither resamples nor re-rates
// the audio: identical sample rate, channel count, and sample count mean the
// same samples play at the same speed and duration.
func VerifyUnchangedAudio(source, output AudioTiming) error {
	if source.SampleRate != output.SampleRate {
		return fmt.Errorf("audio sample rate changed from %d to %d", source.SampleRate, output.SampleRate)
	}
	if source.Channels != output.Channels {
		return fmt.Errorf("audio channel count changed from %d to %d", source.Channels, output.Channels)
	}
	if source.FrameCount != output.FrameCount {
		return fmt.Errorf("audio sample count changed from %d to %d", source.FrameCount, output.FrameCount)
	}
	return nil
}
