// Package httpx contains small helpers for JSON HTTP APIs.
package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maxBodyBytes = 1 << 20 // 1 MiB

// SuccessResponse is the envelope for successful responses.
type SuccessResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
}

// ErrorBody describes an error returned to the client.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ErrorResponse is the envelope for error responses.
type ErrorResponse struct {
	Success bool      `json:"success"`
	Error   ErrorBody `json:"error"`
}

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// OK writes a success envelope.
func OK(w http.ResponseWriter, status int, data any) {
	JSON(w, status, SuccessResponse{Success: true, Data: data})
}

// Error writes an error envelope.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, ErrorResponse{Success: false, Error: ErrorBody{Code: code, Message: message}})
}

// DecodeJSON decodes a size-limited JSON request body into dst.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.Is(err, io.EOF):
			return errors.New("request body is empty")
		case errors.As(err, &maxErr):
			return fmt.Errorf("request body must not exceed %d bytes", maxErr.Limit)
		default:
			return errors.New("request body must be valid JSON")
		}
	}
	return nil
}
