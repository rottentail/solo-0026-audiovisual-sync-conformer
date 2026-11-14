package domain

import (
	"math"
	"strings"
)

const (
	MaxAudioBytes          = 8 << 20
	MaxVideoBytes          = 24 << 20
	MaxRequestBytes        = 46 << 20
	MaxVideoWidth          = 1920
	MaxVideoHeight         = 1080
	MaxVideoFrames         = 1200
	MaxTakeNameRunes       = 120
	MaxTakeNoteRunes       = 500
	DefaultMinConf         = 0.55
	DefaultMaxOffsetMS     = 1000
	DefaultMaxCorrectionMS = 500
	MaxCorrectionMS        = 5000
	MaxFrameRateNum        = 120
	MaxFrameRateDen        = 1001
)

func ValidateCreateTake(input CreateTakeInput) error {
	if err := ValidateIdentifier("take_id", input.TakeID); err != nil {
		return err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return NewError("invalid_request", 400, "name is required")
	}
	if len([]rune(name)) > MaxTakeNameRunes {
		return NewError("invalid_request", 400, "name is too long")
	}
	if len([]rune(input.Note)) > MaxTakeNoteRunes {
		return NewError("invalid_request", 400, "note is too long")
	}
	if input.AudioBase64 == "" || input.VideoBase64 == "" {
		return NewError("invalid_request", 400, "audio_base64 and video_base64 are required")
	}
	return nil
}

func ValidateCreateAnalysis(input CreateAnalysisInput) error {
	if err := ValidateIdentifier("analysis_id", input.AnalysisID); err != nil {
		return err
	}
	if math.IsNaN(input.MinConfidence) || math.IsInf(input.MinConfidence, 0) {
		return NewError("invalid_request", 400, "min_confidence must be finite")
	}
	if input.MinConfidence < 0 || input.MinConfidence > 1 {
		return NewError("invalid_request", 400, "min_confidence must be between 0 and 1")
	}
	if input.MaxOffsetMS < 1 || input.MaxOffsetMS > 10000 {
		return NewError("invalid_request", 400, "max_offset_ms must be between 1 and 10000")
	}
	return nil
}

func ValidateCreateConform(input CreateConformInput) error {
	if err := ValidateIdentifier("conform_id", input.ConformID); err != nil {
		return err
	}
	if input.MaxCorrectionMS < 1 || input.MaxCorrectionMS > MaxCorrectionMS {
		return NewError(
			"invalid_request",
			400,
			"max_correction_ms must be between 1 and %d",
			MaxCorrectionMS,
		)
	}
	return ValidateTargetFrameRate(input.TargetFPSNum, input.TargetFPSDen)
}

// ValidateTargetFrameRate enforces an all-or-nothing rational frame-rate pair.
// A nil pair means no conversion was requested and preserves the historical
// conform behavior. Both members must be present and within the Y4M bounds.
func ValidateTargetFrameRate(num, den *int) error {
	if num == nil && den == nil {
		return nil
	}
	if num == nil || den == nil {
		return NewError(
			"invalid_request",
			400,
			"target_fps_num and target_fps_den must be provided together",
		)
	}
	if *num <= 0 || *num > MaxFrameRateNum || *den <= 0 || *den > MaxFrameRateDen {
		return NewError(
			"invalid_request",
			400,
			"target frame rate must be between 1:%d and %d:1",
			MaxFrameRateDen,
			MaxFrameRateNum,
		)
	}
	return nil
}
