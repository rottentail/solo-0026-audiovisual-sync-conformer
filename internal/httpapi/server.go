package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/domain"
	appservice "github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/service"
)

type API struct {
	service *appservice.Service
}

type createTakeRequest struct {
	TakeID      string `json:"take_id"`
	Name        string `json:"name"`
	Note        string `json:"note"`
	AudioBase64 string `json:"audio_base64"`
	VideoBase64 string `json:"video_base64"`
}

type createAnalysisRequest struct {
	AnalysisID    string   `json:"analysis_id"`
	MinConfidence *float64 `json:"min_confidence"`
	MaxOffsetMS   *int     `json:"max_offset_ms"`
}

type createConformRequest struct {
	ConformID       string `json:"conform_id"`
	MaxCorrectionMS *int   `json:"max_correction_ms"`
}

func New(service *appservice.Service) *API {
	return &API{service: service}
}

func (api *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("POST /v1/takes", api.createTake)
	mux.HandleFunc("GET /v1/takes/{take_id}", api.getTake)
	mux.HandleFunc("POST /v1/takes/{take_id}/sync-analyses", api.createAnalysis)
	mux.HandleFunc("GET /v1/sync-analyses/{analysis_id}", api.getAnalysis)
	mux.HandleFunc("POST /v1/sync-analyses/{analysis_id}/conforms", api.createConform)
	mux.HandleFunc("GET /v1/conforms/{conform_id}", api.getConform)
	mux.HandleFunc("GET /v1/conforms/{conform_id}/bundle", api.getBundle)
	return mux
}

func (api *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (api *API) createTake(w http.ResponseWriter, request *http.Request) {
	var payload createTakeRequest
	if err := decodeJSON(w, request, &payload); err != nil {
		writeFailure(w, err)
		return
	}
	result, err := api.service.CreateTake(domain.CreateTakeInput{
		TakeID:      payload.TakeID,
		Name:        payload.Name,
		Note:        payload.Note,
		AudioBase64: payload.AudioBase64,
		VideoBase64: payload.VideoBase64,
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result.Take)
}

func (api *API) getTake(w http.ResponseWriter, request *http.Request) {
	record, err := api.service.GetTake(request.PathValue("take_id"))
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (api *API) createAnalysis(w http.ResponseWriter, request *http.Request) {
	var payload createAnalysisRequest
	if err := decodeJSON(w, request, &payload); err != nil {
		writeFailure(w, err)
		return
	}
	minConfidence := domain.DefaultMinConf
	if payload.MinConfidence != nil {
		minConfidence = *payload.MinConfidence
	}
	maxOffsetMS := domain.DefaultMaxOffsetMS
	if payload.MaxOffsetMS != nil {
		maxOffsetMS = *payload.MaxOffsetMS
	}
	result, err := api.service.Analyze(request.PathValue("take_id"), domain.CreateAnalysisInput{
		AnalysisID:    payload.AnalysisID,
		MinConfidence: minConfidence,
		MaxOffsetMS:   maxOffsetMS,
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result.Analysis)
}

func (api *API) getAnalysis(w http.ResponseWriter, request *http.Request) {
	record, err := api.service.GetAnalysis(request.PathValue("analysis_id"))
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (api *API) createConform(w http.ResponseWriter, request *http.Request) {
	var payload createConformRequest
	if err := decodeJSON(w, request, &payload); err != nil {
		writeFailure(w, err)
		return
	}
	maxCorrectionMS := domain.DefaultMaxCorrectionMS
	if payload.MaxCorrectionMS != nil {
		maxCorrectionMS = *payload.MaxCorrectionMS
	}
	result, err := api.service.CreateConform(request.PathValue("analysis_id"), domain.CreateConformInput{
		ConformID:       payload.ConformID,
		MaxCorrectionMS: maxCorrectionMS,
	})
	if err != nil {
		writeFailure(w, err)
		return
	}
	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	writeJSON(w, status, result.Conform)
}

func (api *API) getConform(w http.ResponseWriter, request *http.Request) {
	record, err := api.service.GetConform(request.PathValue("conform_id"))
	if err != nil {
		writeFailure(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (api *API) getBundle(w http.ResponseWriter, request *http.Request) {
	id := request.PathValue("conform_id")
	bundle, receipt, err := api.service.ReadConformBundle(id)
	if err != nil {
		writeFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set(
		"Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-review.zip"`, receipt.ConformID),
	)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(bundle)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(bundle)
}

func decodeJSON(w http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(w, request.Body, domain.MaxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		if errors.Is(err, io.EOF) {
			return domain.NewError("invalid_request", 400, "request body is required")
		}
		return domain.NewError("invalid_json", 400, "request body is not valid JSON: %s", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.NewError("invalid_json", 400, "request body must contain one JSON object")
	}
	return nil
}
