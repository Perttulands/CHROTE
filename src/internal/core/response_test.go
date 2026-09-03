package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	data := map[string]string{"test": "data"}

	WriteJSON(recorder, http.StatusOK, data)

	if recorder.Code != http.StatusOK {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusOK)
	}

	contentType := recorder.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, expected application/json", contentType)
	}

	var result map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}
	if result["test"] != "data" {
		t.Errorf("Response body incorrect: %v", result)
	}
}

// The envelope is asserted on the wire rather than on the struct, because the
// serialised shape is what the dashboard reads.
func TestWriteSuccess(t *testing.T) {
	recorder := httptest.NewRecorder()
	data := map[string]string{"result": "ok"}

	WriteSuccess(recorder, data)

	if recorder.Code != http.StatusOK {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusOK)
	}

	var result APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}
	if !result.Success {
		t.Error("Response success should be true")
	}
	if result.Error != nil {
		t.Errorf("Error = %#v, expected nil on a success envelope", result.Error)
	}
	if result.Data == nil {
		t.Error("Data should not be nil")
	}
	if result.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
}

func TestWriteError(t *testing.T) {
	recorder := httptest.NewRecorder()

	WriteError(recorder, http.StatusBadRequest, "BAD_REQUEST", "Invalid input")

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, expected %d", recorder.Code, http.StatusBadRequest)
	}

	var result APIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}
	if result.Success {
		t.Error("Response success should be false")
	}
	if result.Error == nil {
		t.Fatal("Error should not be nil")
	}
	if result.Error.Code != "BAD_REQUEST" {
		t.Errorf("Error code = %q, expected BAD_REQUEST", result.Error.Code)
	}
	if result.Error.Message != "Invalid input" {
		t.Errorf("Error message = %q, expected 'Invalid input'", result.Error.Message)
	}
	if result.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
}

func TestGetErrorStatusCode(t *testing.T) {
	tests := []struct {
		code     string
		expected int
	}{
		{"BAD_REQUEST", http.StatusBadRequest},
		{"FORBIDDEN", http.StatusForbidden},
		{"NOT_FOUND", http.StatusNotFound},
		{"INVALID_JSONL", http.StatusUnprocessableEntity},
		{"BV_NOT_INSTALLED", http.StatusServiceUnavailable},
		{"BV_TIMEOUT", http.StatusGatewayTimeout},
		{"BV_ERROR", http.StatusBadGateway},
		{"BV_INVALID_OUTPUT", http.StatusBadGateway},
		{"ALREADY_RUNNING", http.StatusConflict},
		{"UNKNOWN_ERROR", http.StatusInternalServerError},
		{"", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := GetErrorStatusCode(tt.code)
			if result != tt.expected {
				t.Errorf("GetErrorStatusCode(%q) = %d, expected %d", tt.code, result, tt.expected)
			}
		})
	}
}
