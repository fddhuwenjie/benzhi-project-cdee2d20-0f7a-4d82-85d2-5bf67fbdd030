package httpapi

import (
	"encoding/json"
	"log"
	"net/http"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

type errorResponse struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
		Details   any    `json:"details,omitempty"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("写入 HTTP 响应失败: %v", err)
	}
}

func writeStored(w http.ResponseWriter, result mission.StoredResult, replay bool, requestID string) {
	if requestID != "" {
		w.Header().Set("X-Request-ID", requestID)
	}
	if replay {
		w.Header().Set("Idempotent-Replayed", "true")
		var value map[string]any
		if json.Unmarshal(result.Body, &value) == nil {
			value["idempotent_replay"] = true
			if body, err := json.Marshal(value); err == nil {
				result.Body = body
			}
		}
	}
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)
	_, _ = w.Write([]byte("\n"))
}

func writeError(w http.ResponseWriter, err error, requestID string) {
	domainErr := mission.AsError(err)
	if requestID != "" {
		w.Header().Set("X-Request-ID", requestID)
	}
	var response errorResponse
	response.Error.Code = domainErr.Code
	response.Error.Message = domainErr.Message
	response.Error.RequestID = requestID
	response.Error.Details = domainErr.Details
	writeJSON(w, domainErr.Status, response)
}
