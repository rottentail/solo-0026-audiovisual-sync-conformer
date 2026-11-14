package media

import (
	"fmt"
	"math"
	"sort"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/domain"
)

func AnalyzeSync(audioData, videoData []byte) (domain.Measurement, error) {
	audio, err := InspectAudio(audioData)
	if err != nil {
		return domain.Measurement{}, fmt.Errorf("audio probe failed: %w", err)
	}
	video, err := InspectVideo(videoData)
	if err != nil {
		return domain.Measurement{}, fmt.Errorf("picture probe failed: %w", err)
	}
	audioEvent, audioProminence, err := detectAudioOnset(audio)
	if err != nil {
		return domain.Measurement{}, err
	}
	videoEvent, videoProminence, err := detectPictureFlash(video)
	if err != nil {
		return domain.Measurement{}, err
	}
	offset := audioEvent - videoEvent
	span := maxInt(audioEvent, videoEvent)
	if span < 1 {
		span = 1
	}
	drift := int(math.Round(float64(offset) * 1_000_000 / float64(span*1000)))
	confidence := math.Round(((audioProminence+videoProminence)/2)*1000) / 1000
	return domain.Measurement{
		AudioEventMS:    audioEvent,
		VideoEventMS:    videoEvent,
		OffsetMS:        offset,
		Confidence:      confidence,
		DriftPPM:        drift,
		AudioProminence: audioProminence,
		VideoProminence: videoProminence,
	}, nil
}

func detectAudioOnset(clip AudioClip) (int, float64, error) {
	if len(clip.Samples) == 0 {
		return 0, 0, fmt.Errorf("audio payload has no samples")
	}
	windowFrames := clip.Info.SampleRate / 200
	if windowFrames < 1 {
		windowFrames = 1
	}
	windowSamples := windowFrames * clip.Info.Channels
	windowRMS := make([]float64, 0, len(clip.Samples)/windowSamples)
	for start := 0; start+windowSamples <= len(clip.Samples); start += windowSamples {
		var sum float64
		for _, sample := range clip.Samples[start : start+windowSamples] {
			value := float64(sample)
			sum += value * value
		}
		windowRMS = append(windowRMS, math.Sqrt(sum/float64(windowSamples)))
	}
	if len(windowRMS) < 4 {
		return 0, 0, fmt.Errorf("audio track is too short for onset detection")
	}
	peak := 0.0
	for _, value := range windowRMS {
		if value > peak {
			peak = value
		}
	}
	if peak < 40 {
		return 0, 0, fmt.Errorf("audio onset has insufficient amplitude")
	}
	threshold := peak * 0.18
	minWindow := maxInt(1, 20/5)
	eventWindow := -1
	for index := minWindow; index < len(windowRMS); index++ {
		if windowRMS[index] < threshold {
			continue
		}
		quietNeighbors := 0
		for lookback := 1; lookback <= 4 && index-lookback >= 0; lookback++ {
			if windowRMS[index-lookback] < threshold*0.35 {
				quietNeighbors++
			}
		}
		if quietNeighbors >= 3 {
			eventWindow = index
			break
		}
	}
	if eventWindow < 0 {
		return 0, 0, fmt.Errorf("audio onset was not found")
	}
	startSample := eventWindow * windowSamples
	precise := startSample
	for precise < len(clip.Samples) {
		value := math.Abs(float64(clip.Samples[precise]))
		if value >= peak*0.12 {
			break
		}
		precise++
	}
	eventMS := precise * 1000 / (clip.Info.SampleRate * clip.Info.Channels)
	noiseFloor := 1.0
	quietWindows := minInt(eventWindow, 12)
	if quietWindows > 0 {
		var sum float64
		for _, value := range windowRMS[:quietWindows] {
			sum += value
		}
		noiseFloor = math.Max(1, sum/float64(quietWindows))
	}
	prominence := math.Min(1, math.Log10(1+peak/noiseFloor)/2)
	return eventMS, math.Round(prominence*1000) / 1000, nil
}

func detectPictureFlash(clip VideoClip) (int, float64, error) {
	luma, err := frameLuma(clip)
	if err != nil {
		return 0, 0, err
	}
	baselineValues := append([]float64(nil), luma...)
	sort.Float64s(baselineValues)
	baseline := baselineValues[len(baselineValues)/2]
	maxValue := 0.0
	for _, value := range luma {
		if value > maxValue {
			maxValue = value
		}
	}
	delta := maxValue - baseline
	if delta < 18 {
		return 0, 0, fmt.Errorf("picture flash has insufficient contrast")
	}
	threshold := baseline + delta*0.55
	eventIndex := -1
	for index, value := range luma {
		if value >= threshold {
			eventIndex = index
			break
		}
	}
	if eventIndex < 0 {
		return 0, 0, fmt.Errorf("picture flash was not found")
	}
	eventMS := eventIndex * 1000 * clip.Info.FPSDen / clip.Info.FPSNum
	prominence := math.Min(1, delta/80)
	return eventMS, math.Round(prominence*1000) / 1000, nil
}

// frameLuma computes the mean luma of every frame in display order.
func frameLuma(clip VideoClip) ([]float64, error) {
	if len(clip.Frames) == 0 {
		return nil, fmt.Errorf("picture payload has no frames")
	}
	luma := make([]float64, len(clip.Frames))
	for index, frame := range clip.Frames {
		planeSize := clip.Info.Width * clip.Info.Height
		if len(frame) < planeSize {
			return nil, fmt.Errorf("picture frame is shorter than its luma plane")
		}
		var sum int
		for _, value := range frame[:planeSize] {
			sum += int(value)
		}
		luma[index] = float64(sum) / float64(planeSize)
	}
	return luma, nil
}

// locatePictureFlashes returns the frame indices of the first and last frames
// that clear the flash threshold. A calibration clip carries one flash near the
// clapper and another at the tail, so both indices let a reviewer confirm
// picture timing at the start and at the end of the take.
func locatePictureFlashes(clip VideoClip) (firstIndex, lastIndex int, err error) {
	luma, err := frameLuma(clip)
	if err != nil {
		return 0, 0, err
	}
	baselineValues := append([]float64(nil), luma...)
	sort.Float64s(baselineValues)
	baseline := baselineValues[len(baselineValues)/2]
	maxValue := baselineValues[len(baselineValues)-1]
	delta := maxValue - baseline
	if delta < 18 {
		return 0, 0, fmt.Errorf("picture flash has insufficient contrast")
	}
	threshold := baseline + delta*0.55
	firstIndex, lastIndex = -1, -1
	for index, value := range luma {
		if value >= threshold {
			if firstIndex < 0 {
				firstIndex = index
			}
			lastIndex = index
		}
	}
	if firstIndex < 0 {
		return 0, 0, fmt.Errorf("picture flash was not found")
	}
	return firstIndex, lastIndex, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
