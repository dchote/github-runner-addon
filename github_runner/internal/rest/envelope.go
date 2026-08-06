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

func WriteOK(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(okEnvelope{Result: ResultOK, Data: data})
}

func WriteError(w http.ResponseWriter, status int, code, message string, details interface{}) {
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
