package media

import (
	"fmt"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/domain"
)

// ConvertY4MFrameRate re-times a picture stream to a different constant frame
// rate while preserving its playback duration and frame order.
//
// The output covers the same timeline as the source: output frame count is
// chosen so output duration equals source duration to the nearest output frame
// period, and every output frame is the source frame whose display interval is
// nearest to the output frame center (nearest-center, no interpolation). The
// Y4M header carries the requested rate, so a conforming player shows the same
// length of program rather than a fast or slow replay. Geometry and chroma are
// untouched and the source payload itself is never modified.
func ConvertY4MFrameRate(data []byte, targetFPSNum, targetFPSDen int) ([]byte, domain.VideoInfo, error) {
	if targetFPSNum <= 0 || targetFPSDen <= 0 {
		return nil, domain.VideoInfo{}, fmt.Errorf("target frame rate must be a positive rational")
	}
	if targetFPSNum > domain.MaxFrameRateNum || targetFPSDen > domain.MaxFrameRateDen {
		return nil, domain.VideoInfo{}, fmt.Errorf("target frame rate is outside the accepted range")
	}
	clip, err := InspectVideo(data)
	if err != nil {
		return nil, domain.VideoInfo{}, err
	}
	source := clip.Info
	outputCount, err := outputFrameCount(source.FrameCount, source.FPSNum, source.FPSDen, targetFPSNum, targetFPSDen)
	if err != nil {
		return nil, domain.VideoInfo{}, err
	}
	if outputCount > domain.MaxVideoFrames {
		return nil, domain.VideoInfo{}, fmt.Errorf(
			"converted picture would contain %d frames, exceeding the limit of %d",
			outputCount,
			domain.MaxVideoFrames,
		)
	}

	outputFrames := make([][]byte, outputCount)
	sourceCount := source.FrameCount
	for outputIndex := 0; outputIndex < outputCount; outputIndex++ {
		// Center time of the output frame expressed in source frame units.
		// center = ((2i+1) * Sn * Dd) / (2 * Dn * Sd), rounded to the nearest
		// integer so the chosen source frame is the closest in display time.
		numerator := (2*outputIndex + 1) * source.FPSNum * targetFPSDen
		denominator := 2 * targetFPSNum * source.FPSDen
		sourceIndex := nearestRatio(numerator, denominator)
		if sourceIndex >= sourceCount {
			sourceIndex = sourceCount - 1
		}
		outputFrames[outputIndex] = clip.Frames[sourceIndex]
	}

	outputInfo := domain.VideoInfo{
		Width:      source.Width,
		Height:     source.Height,
		FPSNum:     targetFPSNum,
		FPSDen:     targetFPSDen,
		FrameCount: outputCount,
		DurationMS: outputCount * 1000 * targetFPSDen / targetFPSNum,
		Chroma:     source.Chroma,
	}
	output, err := EncodeY4M(outputInfo, outputFrames)
	if err != nil {
		return nil, domain.VideoInfo{}, err
	}
	outputInfo.Digest = DigestBytes(output)
	outputInfo.ByteCount = len(output)
	return output, outputInfo, nil
}

// outputFrameCount picks the number of output frames whose total duration is
// closest to the source duration. All arithmetic stays integer to keep the
// decision deterministic and free of floating-point drift.
func outputFrameCount(sourceCount, sourceFPSNum, sourceFPSDen, targetFPSNum, targetFPSDen int) (int, error) {
	if sourceCount <= 0 {
		return 0, fmt.Errorf("source picture has no frames")
	}
	// Source duration in abstract ticks: T = sourceCount * Sd / Sn.
	// Output count N satisfies N * Dd / Dn ~= T, hence the ratio below.
	numerator := sourceCount * sourceFPSDen * targetFPSNum
	denominator := sourceFPSNum * targetFPSDen
	if denominator <= 0 || numerator <= 0 {
		return 0, fmt.Errorf("frame-rate ratio is not computable")
	}
	count := nearestRatio(numerator, denominator)
	if count < 1 {
		count = 1
	}
	return count, nil
}

// nearestRatio returns round(numerator/denominator) using half-away-from-zero.
func nearestRatio(numerator, denominator int) int {
	if denominator <= 0 {
		return 0
	}
	quotient := numerator / denominator
	remainder := numerator % denominator
	if remainder < 0 {
		remainder = -remainder
	}
	if remainder*2 >= denominator {
		if numerator >= 0 {
			return quotient + 1
		}
		return quotient - 1
	}
	return quotient
}

// PictureDescriptorFromVideo projects a persisted video descriptor onto the
// receipt/record shape shared by source and output pictures.
func PictureDescriptorFromVideo(info domain.VideoInfo) domain.PictureDescriptor {
	return domain.PictureDescriptor{
		FPSNum:     info.FPSNum,
		FPSDen:     info.FPSDen,
		FrameCount: info.FrameCount,
		DurationMS: info.DurationMS,
		Width:      info.Width,
		Height:     info.Height,
		Chroma:     info.Chroma,
		Digest:     info.Digest,
		ByteCount:  info.ByteCount,
	}
}
