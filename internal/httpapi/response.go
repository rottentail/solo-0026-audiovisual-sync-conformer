package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/rottentail/solo-0026-audiovisual-sync-conformer/internal/domain"
)

type errorEnvelope struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(value)
}

func writeFailure(w http.ResponseWriter, err error) {
	stable := domain.AsError(err)
	status := stable.Status
	if status < 400 || status > 599 {
		status = 500
	}
	writeJSON(w, status, errorEnvelope{
		Error: errorPayload{
			Code:    stable.Code,
			Message: stable.Message,
		},
	})
}
