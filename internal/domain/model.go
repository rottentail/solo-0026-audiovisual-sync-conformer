package domain

import "time"

type AudioInfo struct {
	SampleRate int     `json:"sample_rate"`
	Channels   int     `json:"channels"`
	BitDepth   int     `json:"bit_depth"`
	FrameCount int     `json:"frame_count"`
	DurationMS int     `json:"duration_ms"`
	PeakDBFS   float64 `json:"peak_dbfs"`
	RMSDBFS    float64 `json:"rms_dbfs"`
	Digest     string  `json:"digest"`
	ByteCount  int     `json:"byte_count"`
}

type VideoInfo struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FPSNum     int    `json:"fps_num"`
	FPSDen     int    `json:"fps_den"`
	FrameCount int    `json:"frame_count"`
	DurationMS int    `json:"duration_ms"`
	Chroma     string `json:"chroma"`
	Digest     string `json:"digest"`
	ByteCount  int    `json:"byte_count"`
}

type MediaTake struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Note       string    `json:"note,omitempty"`
	State      string    `json:"state"`
	Audio      AudioInfo `json:"audio"`
	Video      VideoInfo `json:"video"`
	Digest     string    `json:"digest"`
	AdmittedAt time.Time `json:"admitted_at"`
}

type SyncPolicy struct {
	MinConfidence float64 `json:"min_confidence"`
	MaxOffsetMS   int     `json:"max_offset_ms"`
}

type SyncAnalysis struct {
	ID              string     `json:"id"`
	TakeID          string     `json:"take_id"`
	State           string     `json:"state"`
	Policy          SyncPolicy `json:"policy"`
	AudioEventMS    *int       `json:"audio_event_ms,omitempty"`
	VideoEventMS    *int       `json:"video_event_ms,omitempty"`
	OffsetMS        *int       `json:"offset_ms,omitempty"`
	Confidence      float64    `json:"confidence"`
	DriftPPM        int        `json:"drift_ppm"`
	AudioProminence float64    `json:"audio_prominence"`
	VideoProminence float64    `json:"video_prominence"`
	ErrorCode       string     `json:"error_code,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type ConformJob struct {
	ID                string    `json:"id"`
	AnalysisID        string    `json:"analysis_id"`
	TakeID            string    `json:"take_id"`
	State             string    `json:"state"`
	MaxCorrectionMS   int       `json:"max_correction_ms"`
	MeasuredOffsetMS  int       `json:"measured_offset_ms"`
	AppliedOffsetMS   int       `json:"applied_offset_ms"`
	AppliedSamples    int       `json:"applied_samples"`
	AudioOutputDigest string    `json:"audio_output_digest,omitempty"`
	VideoOutputDigest string    `json:"video_output_digest,omitempty"`
	BundleDigest      string    `json:"bundle_digest,omitempty"`
	BundleBytes       int       `json:"bundle_bytes,omitempty"`
	ErrorCode         string    `json:"error_code,omitempty"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type ConformReceipt struct {
	SchemaVersion    int       `json:"schema_version"`
	ConformID        string    `json:"conform_id"`
	AnalysisID       string    `json:"analysis_id"`
	TakeID           string    `json:"take_id"`
	MeasuredOffsetMS int       `json:"measured_offset_ms"`
	AppliedOffsetMS  int       `json:"applied_offset_ms"`
	AppliedSamples   int       `json:"applied_samples"`
	AudioDigest      string    `json:"audio_digest"`
	VideoDigest      string    `json:"video_digest"`
	InputTakeDigest  string    `json:"input_take_digest"`
	CreatedAt        time.Time `json:"created_at"`
}

type CreateTakeInput struct {
	TakeID      string
	Name        string
	Note        string
	AudioBase64 string
	VideoBase64 string
}

type CreateAnalysisInput struct {
	AnalysisID    string
	MinConfidence float64
	MaxOffsetMS   int
}

type CreateConformInput struct {
	ConformID       string
	MaxCorrectionMS int
}

type TakeResult struct {
	Take    MediaTake
	Created bool
}

type AnalysisResult struct {
	Analysis SyncAnalysis
	Created  bool
}

type ConformResult struct {
	Conform ConformJob
	Created bool
}

type Measurement struct {
	AudioEventMS    int
	VideoEventMS    int
	OffsetMS        int
	Confidence      float64
	DriftPPM        int
	AudioProminence float64
	VideoProminence float64
}

type AlignedAudio struct {
	WAV            []byte
	AppliedSamples int
	Digest         string
}
