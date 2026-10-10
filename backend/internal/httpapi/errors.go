package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

// Stable error codes of the mobile-facing API.
const (
	CodeInvalidInput            = "INVALID_INPUT"
	CodeUnsupportedMediaType    = "UNSUPPORTED_MEDIA_TYPE"
	CodePayloadTooLarge         = "PAYLOAD_TOO_LARGE"
	CodeNoRoute                 = "NO_ROUTE"
	CodeOutOfCoverage           = "OUT_OF_COVERAGE"
	CodeLocationNotRoutable     = "LOCATION_NOT_ROUTABLE"
	CodeRoutingUnavailable      = "ROUTING_UNAVAILABLE"
	CodeRoutingTimeout          = "ROUTING_TIMEOUT"
	CodeUpstreamInvalidResponse = "UPSTREAM_INVALID_RESPONSE"
	CodeInternal                = "INTERNAL"
)

type apiError struct {
	status  int
	message string
}

// Indonesian messages are fixed per code; upstream error text is never used.
var apiErrors = map[string]apiError{
	CodeInvalidInput:            {http.StatusBadRequest, "Permintaan tidak valid. Periksa kembali data yang dikirim."},
	CodeUnsupportedMediaType:    {http.StatusUnsupportedMediaType, "Format permintaan harus JSON (Content-Type: application/json)."},
	CodePayloadTooLarge:         {http.StatusRequestEntityTooLarge, "Ukuran permintaan terlalu besar."},
	CodeNoRoute:                 {http.StatusUnprocessableEntity, "Rute tidak ditemukan untuk perjalanan ini."},
	CodeOutOfCoverage:           {http.StatusUnprocessableEntity, "Lokasi berada di luar area layanan saat ini."},
	CodeLocationNotRoutable:     {http.StatusUnprocessableEntity, "Lokasi tidak dapat dijangkau dari jaringan jalan. Coba pilih titik lain di dekatnya."},
	CodeRoutingUnavailable:      {http.StatusServiceUnavailable, "Layanan pencarian rute sedang tidak tersedia. Coba lagi nanti."},
	CodeRoutingTimeout:          {http.StatusGatewayTimeout, "Pencarian rute memakan waktu terlalu lama. Coba lagi."},
	CodeUpstreamInvalidResponse: {http.StatusBadGateway, "Terjadi gangguan pada layanan pencarian rute. Coba lagi nanti."},
	CodeInternal:                {http.StatusInternalServerError, "Terjadi kesalahan pada server."},
}

type errorDetail struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

type errorBody struct {
	Code      string        `json:"code"`
	Message   string        `json:"message"`
	Details   []errorDetail `json:"details,omitempty"`
	RequestID string        `json:"requestId"`
}

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

// writeError writes the error envelope for code and returns the HTTP status.
func writeError(w http.ResponseWriter, requestID, code string, details []errorDetail) int {
	e, ok := apiErrors[code]
	if !ok {
		code, e = CodeInternal, apiErrors[CodeInternal]
	}
	writeJSON(w, e.status, errorEnvelope{Error: errorBody{
		Code: code, Message: e.message, Details: details, RequestID: requestID,
	}})
	return e.status
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// newRequestID returns a random identifier. Client-supplied IDs are not
// accepted so they cannot be used to inject content into logs.
func newRequestID() string {
	var b [12]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	return "req_" + hex.EncodeToString(b[:])
}
