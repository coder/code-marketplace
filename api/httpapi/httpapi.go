package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

type ErrorResponse struct {
	Message   string    `json:"message"`
	Detail    string    `json:"detail"`
	RequestID uuid.UUID `json:"requestId,omitempty"`
}

// WriteBytes tries to write the provided bytes and errors if unable.
func WriteBytes(rw http.ResponseWriter, status int, bytes []byte) {
	rw.WriteHeader(status)
	_, err := rw.Write(bytes)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
}

// Write outputs a standardized format to an HTTP response body.
func Write(rw http.ResponseWriter, status int, response interface{}) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(true)
	err := enc.Encode(response)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	rw.Header().Set("Content-Type", "application/json; charset=utf-8")
	WriteBytes(rw, status, buf.Bytes())
}
