package httpapi

import (
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"pandu-rute/backend/internal/stops"
	"pandu-rute/backend/internal/tripplan"
)

const (
	minStopQuery     = 2   // runes, after trimming
	maxStopQuery     = 100 // runes, after trimming
	defaultStopLimit = 10
	maxStopLimit     = 20

	// stopSourceID names the loaded stops file in dataVersion.
	stopSourceID = "gtfs-stops"
)

type stopResponse struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`

	// Optional display metadata, omitted when unknown.
	Kind            string `json:"kind,omitempty"`
	PlatformCode    string `json:"platformCode,omitempty"`
	ParentStationID string `json:"parentStationId,omitempty"`
}

type stopSearchResponse struct {
	Stops        []stopResponse        `json:"stops"`
	Attributions []attributionResponse `json:"attributions"`
	DataVersion  *dataVersionResponse  `json:"dataVersion"`
}

type stopSearchHandler struct {
	index        *stops.Index
	attributions []tripplan.Attribution
	logger       *slog.Logger
}

func (h *stopSearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := newRequestID()
	w.Header().Set("X-Request-Id", reqID)

	status, code, queryLen, n := h.serve(w, r, reqID)

	// The search text is deliberately not logged, only its length.
	h.logger.Info("stop_search",
		"request_id", reqID,
		"status", status,
		"code", code,
		"query_len", queryLen,
		"results", n,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

func (h *stopSearchHandler) serve(w http.ResponseWriter, r *http.Request, reqID string) (status int, code string, queryLen, results int) {
	q, limit, queryLen, probs := parseStopQuery(r.URL.RawQuery)
	if len(probs) > 0 {
		return writeError(w, reqID, CodeInvalidInput, probs), CodeInvalidInput, queryLen, 0
	}
	// Results are never served without the attribution the data licence requires.
	if h.index == nil || len(h.attributions) == 0 {
		return writeError(w, reqID, CodeStopIndexUnavailable, nil), CodeStopIndexUnavailable, queryLen, 0
	}

	found := h.index.Search(q, limit)
	resp := stopSearchResponse{
		Stops:        make([]stopResponse, 0, len(found)),
		Attributions: make([]attributionResponse, 0, len(h.attributions)),
	}
	for _, s := range found {
		resp.Stops = append(resp.Stops, stopResponse{
			ID: s.ID, Name: s.Name, Lat: s.Lat, Lon: s.Lon,
			Kind: string(s.Kind), PlatformCode: s.PlatformCode, ParentStationID: s.ParentStationID,
		})
	}
	for _, a := range h.attributions {
		resp.Attributions = append(resp.Attributions, attributionResponse{
			ID: a.ID, Text: a.Text, License: a.License, LicenseURL: a.LicenseURL, SourceURL: a.SourceURL,
		})
	}
	if v := h.index.Version(); v != "" {
		resp.DataVersion = &dataVersionResponse{Sources: []sourceVersionResponse{{ID: stopSourceID, SHA256: v}}}
	}
	writeJSON(w, http.StatusOK, resp)
	return http.StatusOK, "", queryLen, len(found)
}

// parseStopQuery validates q and limit. Unknown and repeated parameters are
// rejected, consistent with the strict trip-plan request decoding.
func parseStopQuery(raw string) (q string, limit, queryLen int, probs []errorDetail) {
	vals, err := url.ParseQuery(raw)
	if err != nil {
		return "", 0, 0, []errorDetail{{"query", tripplan.ReasonInvalidFormat}}
	}
	var unknown []string
	for k := range vals {
		if k != "q" && k != "limit" {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		probs = append(probs, errorDetail{k, tripplan.ReasonUnknownField})
	}

	switch qs := vals["q"]; {
	case len(qs) > 1:
		probs = append(probs, errorDetail{"q", tripplan.ReasonInvalidFormat})
	case len(qs) == 0 || strings.TrimSpace(qs[0]) == "":
		probs = append(probs, errorDetail{"q", tripplan.ReasonRequired})
	default:
		q = strings.TrimSpace(qs[0])
		queryLen = utf8.RuneCountInString(q)
		switch {
		case queryLen < minStopQuery:
			probs = append(probs, errorDetail{"q", tripplan.ReasonTooShort})
		case queryLen > maxStopQuery:
			probs = append(probs, errorDetail{"q", tripplan.ReasonTooLong})
		case strings.IndexFunc(q, unicode.IsControl) >= 0:
			probs = append(probs, errorDetail{"q", tripplan.ReasonInvalidFormat})
		}
	}

	limit = defaultStopLimit
	switch ls := vals["limit"]; {
	case len(ls) > 1:
		probs = append(probs, errorDetail{"limit", tripplan.ReasonInvalidFormat})
	case len(ls) == 1:
		n, err := strconv.Atoi(ls[0])
		switch {
		case err != nil:
			probs = append(probs, errorDetail{"limit", tripplan.ReasonInvalidFormat})
		case n < 1 || n > maxStopLimit:
			probs = append(probs, errorDetail{"limit", tripplan.ReasonOutOfRange})
		default:
			limit = n
		}
	}
	return q, limit, queryLen, probs
}
