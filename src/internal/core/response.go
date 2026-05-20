// Package core provides business logic and utility functions
package core

import (
	"encoding/json"
	"net/http"
	"time"
)

// APIError represents an error response
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIResponse is the standard response format
type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *APIError   `json:"error,omitempty"`
	Timestamp string      `json:"timestamp"`
}

// NewSuccessResponse creates a success response
func NewSuccessResponse(data interface{}) APIResponse {
	return APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// NewErrorResponse creates an error response
func NewErrorResponse(code, message string) APIResponse {
	return APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// WriteJSON writes a JSON response
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteSuccess writes a success JSON response
func WriteSuccess(w http.ResponseWriter, data interface{}) {
	WriteJSON(w, http.StatusOK, NewSuccessResponse(data))
}

// WriteError writes an error JSON response
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, NewErrorResponse(code, message))
}

// GetErrorStatusCode maps error codes to HTTP status codes
func GetErrorStatusCode(code string) int {
	switch code {
	case "BAD_REQUEST":
		return http.StatusBadRequest
	case "FORBIDDEN":
		return http.StatusForbidden
	case "NOT_FOUND":
		return http.StatusNotFound
	case "INVALID_JSONL":
		return http.StatusUnprocessableEntity
	case "BV_NOT_INSTALLED":
		return http.StatusServiceUnavailable
	case "BV_TIMEOUT":
		return http.StatusGatewayTimeout
	case "BV_ERROR", "BV_INVALID_OUTPUT":
		return http.StatusBadGateway
	case "ALREADY_RUNNING":
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}
