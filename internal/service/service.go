package service

import (
	"encoding/base64"
	"fmt"
	"time"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/domain"
	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/media"
	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/storage"
)

type Service struct {
	repository *storage.Repository
}

func New(repository *storage.Repository) *Service {
	return &Service{repository: repository}
}

func (s *Service) Repository() *storage.Repository {
	return s.repository
}

func (s *Service) CreateTake(input domain.CreateTakeInput) (domain.TakeResult, error) {
	if err := domain.ValidateCreateTake(input); err != nil {
		return domain.TakeResult{}, err
	}
	audio, err := decodePayload("audio_base64", input.AudioBase64, domain.MaxAudioBytes)
	if err != nil {
		return domain.TakeResult{}, err
	}
	video, err := decodePayload("video_base64", input.VideoBase64, domain.MaxVideoBytes)
	if err != nil {
		return domain.TakeResult{}, err
	}
	audioClip, err := media.InspectAudio(audio)
	if err != nil {
		return domain.TakeResult{}, domain.NewError("invalid_audio", 422, "%s", err)
	}
	videoClip, err := media.InspectVideo(video)
	if err != nil {
		return domain.TakeResult{}, domain.NewError("invalid_video", 422, "%s", err)
	}
	now := time.Now().UTC()
	record := domain.MediaTake{
		ID:         input.TakeID,
		Name:       input.Name,
		Note:       input.Note,
		State:      domain.TakeStateAdmitted,
		Audio:      audioClip.Info,
		Video:      videoClip.Info,
		AdmittedAt: now,
	}
	record.Digest = media.CombineDigest(
		record.Name,
		record.Note,
		record.Audio.Digest,
		record.Video.Digest,
	)
	admitted, created, err := s.repository.AdmitTake(record, audio, video)
	if err != nil {
		return domain.TakeResult{}, err
	}
	return domain.TakeResult{Take: admitted, Created: created}, nil
}

func (s *Service) GetTake(id string) (domain.MediaTake, error) {
	if err := domain.ValidateIdentifier("take_id", id); err != nil {
		return domain.MediaTake{}, err
	}
	return s.repository.GetTake(id)
}

func (s *Service) Analyze(takeID string, input domain.CreateAnalysisInput) (domain.AnalysisResult, error) {
	if err := domain.ValidateIdentifier("take_id", takeID); err != nil {
		return domain.AnalysisResult{}, err
	}
	if err := domain.ValidateCreateAnalysis(input); err != nil {
		return domain.AnalysisResult{}, err
	}
	take, err := s.repository.GetTake(takeID)
	if err != nil {
		return domain.AnalysisResult{}, err
	}
	now := time.Now().UTC()
	record := domain.SyncAnalysis{
		ID:        input.AnalysisID,
		TakeID:    take.ID,
		State:     domain.AnalysisStateQueued,
		Policy:    domain.SyncPolicy{MinConfidence: input.MinConfidence, MaxOffsetMS: input.MaxOffsetMS},
		CreatedAt: now,
		UpdatedAt: now,
	}
	existing, created, err := s.repository.CreateAnalysis(record)
	if err != nil {
		return domain.AnalysisResult{}, err
	}
	if !created {
		return domain.AnalysisResult{Analysis: existing, Created: false}, nil
	}
	record.State = domain.AnalysisStateDetecting
	record.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateAnalysis(record); err != nil {
		return domain.AnalysisResult{}, err
	}

	audio, video, err := s.repository.ReadTakeMedia(take.ID)
	if err != nil {
		return s.failAnalysis(record, "take_media_unavailable", err.Error())
	}
	measurement, err := media.AnalyzeSync(audio, video)
	if err != nil {
		return s.failAnalysis(record, "sync_detection_failed", err.Error())
	}
	record.AudioEventMS = intPointer(measurement.AudioEventMS)
	record.VideoEventMS = intPointer(measurement.VideoEventMS)
	record.OffsetMS = intPointer(measurement.OffsetMS)
	record.Confidence = measurement.Confidence
	record.DriftPPM = measurement.DriftPPM
	record.AudioProminence = measurement.AudioProminence
	record.VideoProminence = measurement.VideoProminence
	record.UpdatedAt = time.Now().UTC()

	if measurement.Confidence < record.Policy.MinConfidence {
		return s.failAnalysis(
			record,
			"low_confidence",
			fmt.Sprintf(
				"confidence %.3f is below the minimum %.3f",
				measurement.Confidence,
				record.Policy.MinConfidence,
			),
		)
	}
	if abs(measurement.OffsetMS) > record.Policy.MaxOffsetMS {
		return s.failAnalysis(
			record,
			"offset_exceeded",
			fmt.Sprintf(
				"offset %d ms exceeds the analysis limit of %d ms",
				abs(measurement.OffsetMS),
				record.Policy.MaxOffsetMS,
			),
		)
	}
	record.State = domain.AnalysisStateReady
	if err := s.repository.UpdateAnalysis(record); err != nil {
		return domain.AnalysisResult{}, err
	}
	return domain.AnalysisResult{Analysis: record, Created: true}, nil
}

func (s *Service) failAnalysis(
	record domain.SyncAnalysis,
	code string,
	message string,
) (domain.AnalysisResult, error) {
	record.State = domain.AnalysisStateFailed
	record.ErrorCode = code
	record.ErrorMessage = message
	record.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateAnalysis(record); err != nil {
		return domain.AnalysisResult{}, err
	}
	return domain.AnalysisResult{Analysis: record, Created: true}, nil
}

func (s *Service) GetAnalysis(id string) (domain.SyncAnalysis, error) {
	if err := domain.ValidateIdentifier("analysis_id", id); err != nil {
		return domain.SyncAnalysis{}, err
	}
	return s.repository.GetAnalysis(id)
}

func (s *Service) CreateConform(
	analysisID string,
	input domain.CreateConformInput,
) (domain.ConformResult, error) {
	if err := domain.ValidateIdentifier("analysis_id", analysisID); err != nil {
		return domain.ConformResult{}, err
	}
	if err := domain.ValidateCreateConform(input); err != nil {
		return domain.ConformResult{}, err
	}
	analysis, err := s.repository.GetAnalysis(analysisID)
	if err != nil {
		return domain.ConformResult{}, err
	}
	if analysis.State != domain.AnalysisStateReady || analysis.OffsetMS == nil {
		return domain.ConformResult{}, domain.NewError(
			"analysis_not_ready",
			409,
			"analysis %s is not ready for a conform",
			analysisID,
		)
	}
	targetFPSNum, targetFPSDen := 0, 0
	if input.TargetFPSNum != nil && input.TargetFPSDen != nil {
		targetFPSNum = *input.TargetFPSNum
		targetFPSDen = *input.TargetFPSDen
	}
	record := domain.ConformJob{
		ID:               input.ConformID,
		AnalysisID:       analysis.ID,
		TakeID:           analysis.TakeID,
		State:            domain.ConformStateQueued,
		MaxCorrectionMS:  input.MaxCorrectionMS,
		TargetFPSNum:     targetFPSNum,
		TargetFPSDen:     targetFPSDen,
		MeasuredOffsetMS: *analysis.OffsetMS,
		CreatedAt:        time.Now().UTC(),
		UpdatedAt:        time.Now().UTC(),
	}
	existing, created, err := s.repository.CreateConform(record)
	if err != nil {
		return domain.ConformResult{}, err
	}
	if !created {
		return domain.ConformResult{Conform: existing, Created: false}, nil
	}
	if abs(record.MeasuredOffsetMS) > record.MaxCorrectionMS {
		record.State = domain.ConformStateFailed
		record.ErrorCode = "correction_limit_exceeded"
		record.ErrorMessage = fmt.Sprintf(
			"measured correction of %d ms exceeds the ceiling of %d ms",
			abs(record.MeasuredOffsetMS),
			record.MaxCorrectionMS,
		)
		record.UpdatedAt = time.Now().UTC()
		if err := s.repository.UpdateConform(record); err != nil {
			return domain.ConformResult{}, err
		}
		return domain.ConformResult{Conform: record, Created: true}, domain.NewError(
			record.ErrorCode,
			422,
			record.ErrorMessage,
		)
	}
	record.State = domain.ConformStateRendering
	record.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateConform(record); err != nil {
		return domain.ConformResult{}, err
	}
	return s.renderConform(record)
}

func (s *Service) renderConform(record domain.ConformJob) (domain.ConformResult, error) {
	audio, video, err := s.repository.ReadTakeMedia(record.TakeID)
	if err != nil {
		return s.failConform(record, "take_media_unavailable", err.Error())
	}
	alignedAudio, err := media.AlignAudio(audio, record.MeasuredOffsetMS, record.MaxCorrectionMS)
	if err != nil {
		return s.failConform(record, domain.AsError(err).Code, domain.AsError(err).Message)
	}
	take, err := s.repository.GetTake(record.TakeID)
	if err != nil {
		return s.failConform(record, "take_media_unavailable", err.Error())
	}

	conversionRequested := record.TargetFPSNum > 0 && record.TargetFPSDen > 0
	var conformedVideo []byte
	var videoInfo domain.VideoInfo
	if conversionRequested {
		conformedVideo, videoInfo, err = media.ConvertY4MFrameRate(
			video,
			record.TargetFPSNum,
			record.TargetFPSDen,
		)
		if err != nil {
			return s.failConform(record, "frame_rate_conversion_failed", err.Error())
		}
	} else {
		// Legacy path: re-emit the source geometry and frame rate unchanged.
		conformedVideo, videoInfo, err = media.NormalizeY4M(video)
		if err != nil {
			return s.failConform(record, "video_conform_failed", err.Error())
		}
	}

	record.AppliedOffsetMS = record.MeasuredOffsetMS
	record.AppliedSamples = alignedAudio.AppliedSamples
	record.AudioOutputDigest = alignedAudio.Digest
	record.VideoOutputDigest = videoInfo.Digest
	record.SourcePicture = media.PictureDescriptorFromVideo(take.Video)
	record.OutputPicture = media.PictureDescriptorFromVideo(videoInfo)
	// The receipt flag reflects the actual delivered picture, not merely the
	// request: a target equal to the source rate re-times nothing.
	rateConverted := videoInfo.FPSNum != take.Video.FPSNum || videoInfo.FPSDen != take.Video.FPSDen
	receipt := domain.ConformReceipt{
		SchemaVersion:      2,
		ConformID:          record.ID,
		AnalysisID:         record.AnalysisID,
		TakeID:             record.TakeID,
		MeasuredOffsetMS:   record.MeasuredOffsetMS,
		AppliedOffsetMS:    record.AppliedOffsetMS,
		AppliedSamples:     record.AppliedSamples,
		AudioDigest:        alignedAudio.Digest,
		VideoDigest:        videoInfo.Digest,
		InputTakeDigest:    take.Digest,
		FrameRateConverted: rateConverted,
		SourcePicture:      record.SourcePicture,
		OutputPicture:      record.OutputPicture,
		CreatedAt:          record.CreatedAt,
	}
	bundle, receiptData, err := media.BuildReviewBundle(alignedAudio.WAV, conformedVideo, receipt)
	if err != nil {
		return s.failConform(record, "bundle_build_failed", err.Error())
	}
	record.BundleDigest = media.DigestBytes(bundle)
	record.BundleBytes = len(bundle)
	record.State = domain.ConformStateCommitting
	record.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateConform(record); err != nil {
		return domain.ConformResult{}, err
	}
	completed, err := s.repository.CommitConform(record, bundle, receiptData)
	if err != nil {
		return s.failConform(record, "bundle_commit_failed", err.Error())
	}
	return domain.ConformResult{Conform: completed, Created: true}, nil
}

func (s *Service) failConform(
	record domain.ConformJob,
	code string,
	message string,
) (domain.ConformResult, error) {
	record.State = domain.ConformStateFailed
	record.ErrorCode = code
	record.ErrorMessage = message
	record.UpdatedAt = time.Now().UTC()
	if err := s.repository.UpdateConform(record); err != nil {
		return domain.ConformResult{}, err
	}
	return domain.ConformResult{Conform: record, Created: true}, domain.NewError(code, 422, message)
}

func (s *Service) GetConform(id string) (domain.ConformJob, error) {
	if err := domain.ValidateIdentifier("conform_id", id); err != nil {
		return domain.ConformJob{}, err
	}
	return s.repository.GetConform(id)
}

func (s *Service) ReadConformBundle(id string) ([]byte, domain.ConformReceipt, error) {
	if err := domain.ValidateIdentifier("conform_id", id); err != nil {
		return nil, domain.ConformReceipt{}, err
	}
	return s.repository.ReadConformBundle(id)
}

func decodePayload(field, encoded string, maximum int) ([]byte, error) {
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return nil, domain.NewError("invalid_base64", 400, "%s is not valid base64", field)
	}
	if len(data) == 0 {
		return nil, domain.NewError("invalid_request", 400, "%s is empty", field)
	}
	if len(data) > maximum {
		return nil, domain.NewError(
			"payload_too_large",
			413,
			"%s exceeds the %d byte limit",
			field,
			maximum,
		)
	}
	return data, nil
}

func intPointer(value int) *int {
	return &value
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
