package rest

import (
	"encoding/json"
	"net/http"
)

const (
	ResultOK    = "ok"
	ResultError = "error"
)

type ErrorBody struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details"`
}

type okEnvelope struct {
	Result string      `json:"result"`
	Data   interface{} `json:"data"`
}

type errEnvelope struct {
	Result string    `json:"result"`
	Error  ErrorBody `json:"error"`
}

// headerWritten reports whether WriteHeader was already called (e.g. timeout 504).
// Walks Unwrap() so chi Timeout (and similar) wrappers still find onceHeaderWriter.
func headerWritten(w http.ResponseWriter) bool {
	for w != nil {
		if ow, ok := w.(*onceHeaderWriter); ok {
			return ow.wrote.Load()
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return false
		}
		w = u.Unwrap()
	}
	return false
}

func WriteOK(w http.ResponseWriter, status int, data interface{}) {
	if headerWritten(w) {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(okEnvelope{Result: ResultOK, Data: data})
}

func WriteError(w http.ResponseWriter, status int, code, message string, details interface{}) {
	if headerWritten(w) {
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if details == nil {
		details = map[string]interface{}{}
	}
	_ = json.NewEncoder(w).Encode(errEnvelope{
		Result: ResultError,
		Error: ErrorBody{
			Code:    code,
			Message: message,
			Details: details,
		},
	})
}
