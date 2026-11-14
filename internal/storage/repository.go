package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/domain"
)

type Repository struct {
	root  string
	locks *keyLocks
}

func New(root string) (*Repository, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve data root: %w", err)
	}
	for _, name := range []string{"takes", "analyses", "conforms", "conforms/assets", "tmp"} {
		if err := os.MkdirAll(filepath.Join(absolute, name), 0o750); err != nil {
			return nil, fmt.Errorf("create data namespace %s: %w", name, err)
		}
	}
	return &Repository{root: absolute, locks: newKeyLocks()}, nil
}

func (r *Repository) Root() string {
	return r.root
}

func (r *Repository) takeDir(id string) string {
	return filepath.Join(r.root, "takes", id)
}

func (r *Repository) takeRecordPath(id string) string {
	return filepath.Join(r.takeDir(id), "record.json")
}

func (r *Repository) analysisPath(id string) string {
	return filepath.Join(r.root, "analyses", id+".json")
}

func (r *Repository) conformPath(id string) string {
	return filepath.Join(r.root, "conforms", id+".json")
}

func (r *Repository) bundleDir(id string) string {
	return filepath.Join(r.root, "conforms", "assets", id)
}

func (r *Repository) GetTake(id string) (domain.MediaTake, error) {
	var record domain.MediaTake
	if err := readJSON(r.takeRecordPath(id), &record); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.MediaTake{}, domain.NewError("take_not_found", 404, "take %s does not exist", id)
		}
		return domain.MediaTake{}, fmt.Errorf("read take: %w", err)
	}
	return record, nil
}

func (r *Repository) AdmitTake(
	record domain.MediaTake,
	audio []byte,
	video []byte,
) (domain.MediaTake, bool, error) {
	lock := r.locks.forKey("take:" + record.ID)
	lock.Lock()
	defer lock.Unlock()

	existing, err := r.GetTake(record.ID)
	switch {
	case err == nil:
		if existing.Name == record.Name &&
			existing.Note == record.Note &&
			existing.Audio.Digest == record.Audio.Digest &&
			existing.Video.Digest == record.Video.Digest {
			return existing, false, nil
		}
		return domain.MediaTake{}, false, domain.NewError(
			"take_conflict",
			409,
			"take %s already exists with different media or metadata",
			record.ID,
		)
	case !domain.IsErrorCode(err, "take_not_found"):
		return domain.MediaTake{}, false, err
	}

	tempDir, err := os.MkdirTemp(filepath.Join(r.root, "tmp"), "take-"+record.ID+"-*")
	if err != nil {
		return domain.MediaTake{}, false, fmt.Errorf("create take staging directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := writeFileAtomic(filepath.Join(tempDir, "audio.wav"), audio); err != nil {
		return domain.MediaTake{}, false, err
	}
	if err := writeFileAtomic(filepath.Join(tempDir, "picture.y4m"), video); err != nil {
		return domain.MediaTake{}, false, err
	}
	recordData, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return domain.MediaTake{}, false, fmt.Errorf("encode take record: %w", err)
	}
	recordData = append(recordData, '\n')
	if err := writeFileAtomic(filepath.Join(tempDir, "record.json"), recordData); err != nil {
		return domain.MediaTake{}, false, err
	}
	if err := syncDirectory(tempDir); err != nil {
		return domain.MediaTake{}, false, fmt.Errorf("sync take staging directory: %w", err)
	}
	if err := os.Rename(tempDir, r.takeDir(record.ID)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return domain.MediaTake{}, false, domain.NewError(
				"take_conflict",
				409,
				"take %s was created concurrently",
				record.ID,
			)
		}
		return domain.MediaTake{}, false, fmt.Errorf("commit take directory: %w", err)
	}
	return record, true, nil
}

func (r *Repository) ReadTakeMedia(id string) ([]byte, []byte, error) {
	audio, err := os.ReadFile(filepath.Join(r.takeDir(id), "audio.wav"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, domain.NewError("take_not_found", 404, "take %s does not exist", id)
		}
		return nil, nil, fmt.Errorf("read take audio: %w", err)
	}
	video, err := os.ReadFile(filepath.Join(r.takeDir(id), "picture.y4m"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, domain.NewError("take_media_missing", 500, "take %s picture is missing", id)
		}
		return nil, nil, fmt.Errorf("read take picture: %w", err)
	}
	return audio, video, nil
}

func (r *Repository) GetAnalysis(id string) (domain.SyncAnalysis, error) {
	var record domain.SyncAnalysis
	if err := readJSON(r.analysisPath(id), &record); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.SyncAnalysis{}, domain.NewError(
				"analysis_not_found",
				404,
				"analysis %s does not exist",
				id,
			)
		}
		return domain.SyncAnalysis{}, fmt.Errorf("read analysis: %w", err)
	}
	return record, nil
}

func (r *Repository) CreateAnalysis(record domain.SyncAnalysis) (domain.SyncAnalysis, bool, error) {
	lock := r.locks.forKey("analysis:" + record.ID)
	lock.Lock()
	defer lock.Unlock()
	existing, err := r.GetAnalysis(record.ID)
	switch {
	case err == nil:
		if existing.TakeID == record.TakeID && existing.Policy == record.Policy {
			return existing, false, nil
		}
		return domain.SyncAnalysis{}, false, domain.NewError(
			"analysis_conflict",
			409,
			"analysis %s already exists with a different take or policy",
			record.ID,
		)
	case !domain.IsErrorCode(err, "analysis_not_found"):
		return domain.SyncAnalysis{}, false, err
	}
	if err := r.writeAnalysis(record); err != nil {
		return domain.SyncAnalysis{}, false, err
	}
	return record, true, nil
}

func (r *Repository) UpdateAnalysis(record domain.SyncAnalysis) error {
	lock := r.locks.forKey("analysis:" + record.ID)
	lock.Lock()
	defer lock.Unlock()
	current, err := r.GetAnalysis(record.ID)
	if err != nil {
		return err
	}
	if !domain.CanAdvanceAnalysis(current.State, record.State) {
		return domain.NewError(
			"analysis_transition_invalid",
			409,
			"analysis cannot move from %s to %s",
			current.State,
			record.State,
		)
	}
	return r.writeAnalysis(record)
}

func (r *Repository) writeAnalysis(record domain.SyncAnalysis) error {
	return writeJSON(r.analysisPath(record.ID), record)
}

func (r *Repository) GetConform(id string) (domain.ConformJob, error) {
	var record domain.ConformJob
	if err := readJSON(r.conformPath(id), &record); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.ConformJob{}, domain.NewError(
				"conform_not_found",
				404,
				"conform %s does not exist",
				id,
			)
		}
		return domain.ConformJob{}, fmt.Errorf("read conform: %w", err)
	}
	return record, nil
}

func (r *Repository) CreateConform(record domain.ConformJob) (domain.ConformJob, bool, error) {
	lock := r.locks.forKey("conform:" + record.ID)
	lock.Lock()
	defer lock.Unlock()
	existing, err := r.GetConform(record.ID)
	switch {
	case err == nil:
		if existing.AnalysisID == record.AnalysisID &&
			existing.MaxCorrectionMS == record.MaxCorrectionMS &&
			existing.TargetFPSNum == record.TargetFPSNum &&
			existing.TargetFPSDen == record.TargetFPSDen {
			return existing, false, nil
		}
		return domain.ConformJob{}, false, domain.NewError(
			"conform_conflict",
			409,
			"conform %s already exists with a different analysis, correction ceiling, or target frame rate",
			record.ID,
		)
	case !domain.IsErrorCode(err, "conform_not_found"):
		return domain.ConformJob{}, false, err
	}
	if err := r.writeConform(record); err != nil {
		return domain.ConformJob{}, false, err
	}
	return record, true, nil
}

func (r *Repository) UpdateConform(record domain.ConformJob) error {
	lock := r.locks.forKey("conform:" + record.ID)
	lock.Lock()
	defer lock.Unlock()
	current, err := r.GetConform(record.ID)
	if err != nil {
		return err
	}
	if !domain.CanAdvanceConform(current.State, record.State) {
		return domain.NewError(
			"conform_transition_invalid",
			409,
			"conform cannot move from %s to %s",
			current.State,
			record.State,
		)
	}
	return r.writeConform(record)
}

func (r *Repository) CommitConform(
	record domain.ConformJob,
	bundle []byte,
	receipt []byte,
) (domain.ConformJob, error) {
	lock := r.locks.forKey("conform:" + record.ID)
	lock.Lock()
	defer lock.Unlock()
	current, err := r.GetConform(record.ID)
	if err != nil {
		return domain.ConformJob{}, err
	}
	if !domain.CanAdvanceConform(current.State, domain.ConformStateCompleted) {
		return domain.ConformJob{}, domain.NewError(
			"conform_transition_invalid",
			409,
			"conform cannot complete from %s",
			current.State,
		)
	}
	tempDir, err := os.MkdirTemp(filepath.Join(r.root, "tmp"), "conform-"+record.ID+"-*")
	if err != nil {
		return domain.ConformJob{}, fmt.Errorf("create bundle staging directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	if err := writeFileAtomic(filepath.Join(tempDir, "bundle.zip"), bundle); err != nil {
		return domain.ConformJob{}, err
	}
	if err := writeFileAtomic(filepath.Join(tempDir, "receipt.json"), receipt); err != nil {
		return domain.ConformJob{}, err
	}
	if err := syncDirectory(tempDir); err != nil {
		return domain.ConformJob{}, fmt.Errorf("sync bundle staging directory: %w", err)
	}
	finalDir := r.bundleDir(record.ID)
	if err := os.Rename(tempDir, finalDir); err != nil {
		if errors.Is(err, os.ErrExist) {
			return domain.ConformJob{}, domain.NewError(
				"conform_bundle_conflict",
				409,
				"bundle %s already exists",
				record.ID,
			)
		}
		return domain.ConformJob{}, fmt.Errorf("commit bundle directory: %w", err)
	}
	record.State = domain.ConformStateCompleted
	record.UpdatedAt = time.Now().UTC()
	if err := r.writeConform(record); err != nil {
		os.RemoveAll(finalDir)
		return domain.ConformJob{}, err
	}
	return record, nil
}

func (r *Repository) ReadConformBundle(id string) ([]byte, domain.ConformReceipt, error) {
	record, err := r.GetConform(id)
	if err != nil {
		return nil, domain.ConformReceipt{}, err
	}
	if record.State != domain.ConformStateCompleted {
		return nil, domain.ConformReceipt{}, domain.NewError(
			"conform_not_ready",
			409,
			"conform %s is not completed",
			id,
		)
	}
	bundle, err := os.ReadFile(filepath.Join(r.bundleDir(id), "bundle.zip"))
	if err != nil {
		return nil, domain.ConformReceipt{}, fmt.Errorf("read conform bundle: %w", err)
	}
	if len(bundle) != record.BundleBytes || shaOrEmpty(bundle) != record.BundleDigest {
		return nil, domain.ConformReceipt{}, domain.NewError(
			"bundle_integrity_failed",
			500,
			"stored bundle does not match its completed record",
		)
	}
	var receipt domain.ConformReceipt
	if err := readJSON(filepath.Join(r.bundleDir(id), "receipt.json"), &receipt); err != nil {
		return nil, domain.ConformReceipt{}, fmt.Errorf("read conform receipt: %w", err)
	}
	return bundle, receipt, nil
}

func (r *Repository) writeConform(record domain.ConformJob) error {
	return writeJSON(r.conformPath(record.ID), record)
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON record: %w", err)
	}
	data = append(data, '\n')
	return writeFileAtomic(path, data)
}

func readJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode JSON record: %w", err)
	}
	return nil
}
