package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

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

const (
	ForwardedHeader       = "Forwarded"
	XForwardedHostHeader  = "X-Forwarded-Host"
	XForwardedProtoHeader = "X-Forwarded-Proto"
)

// RequestBaseURL returns the base URL of the request.  It prioritizes
// forwarded proxy headers.
func RequestBaseURL(r *http.Request, basePath string) url.URL {
	proto := ""
	host := ""

	// by=<identifier>;for=<identifier>;host=<host>;proto=<http|https>
	forwarded := strings.Split(r.Header.Get(ForwardedHeader), ";")
	for _, val := range forwarded {
		parts := strings.SplitN(val, "=", 2)
		switch strings.TrimSpace(parts[0]) {
		case "host":
			host = strings.TrimSpace(parts[1])
		case "proto":
			proto = strings.TrimSpace(parts[1])
		}
	}

	if proto == "" {
		proto = r.Header.Get(XForwardedProtoHeader)
	}
	if proto == "" {
		proto = "http"
	}

	if host == "" {
		host = r.Header.Get(XForwardedHostHeader)
	}
	if host == "" {
		host = r.Host
	}

	return url.URL{
		Scheme: proto,
		Host:   host,
		Path:   strings.TrimRight(basePath, "/"),
	}
}
