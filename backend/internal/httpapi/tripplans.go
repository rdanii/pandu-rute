package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"mime"
	"net/http"
	"strings"
	"time"

	"pandu-rute/backend/internal/tripplan"
)

const maxTripPlanBody = 4 << 10 // 4 KiB

// wib is Western Indonesia Time. Indonesia has no daylight saving time, so a
// fixed zone avoids depending on the host's tz database.
var wib = time.FixedZone("WIB", 7*60*60)

// --- Request DTO ---------------------------------------------------------

type locationRequest struct {
	Lat   *float64 `json:"lat"`
	Lon   *float64 `json:"lon"`
	Label *string  `json:"label"`
}

type tripPlanRequest struct {
	Origin         *locationRequest `json:"origin"`
	Destination    *locationRequest `json:"destination"`
	DepartAt       *string          `json:"departAt"`
	MaxItineraries *int             `json:"maxItineraries"`
}

// --- Response DTO --------------------------------------------------------

type locationResponse struct {
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
	Label string  `json:"label,omitempty"`
}

type placeResponse struct {
	StopID string  `json:"stopId,omitempty"`
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}

type lineResponse struct {
	ID        string `json:"id"`
	ShortName string `json:"shortName"`
	LongName  string `json:"longName,omitempty"`
}

type legResponse struct {
	Type            string        `json:"type"`
	Mode            string        `json:"mode"`
	Line            *lineResponse `json:"line,omitempty"`
	Headsign        string        `json:"headsign,omitempty"`
	From            placeResponse `json:"from"`
	To              placeResponse `json:"to"`
	DurationSeconds int64         `json:"durationSeconds"`
	DistanceMeters  int64         `json:"distanceMeters"`
}

type itineraryResponse struct {
	Departure          string        `json:"departure"`
	Arrival            string        `json:"arrival"`
	TimeQuality        string        `json:"timeQuality"`
	DurationSeconds    int64         `json:"durationSeconds"`
	Transfers          int           `json:"transfers"`
	WalkDistanceMeters int64         `json:"walkDistanceMeters"`
	Legs               []legResponse `json:"legs"`
}

type attributionResponse struct {
	ID         string `json:"id"`
	Text       string `json:"text"`
	License    string `json:"license,omitempty"`
	LicenseURL string `json:"licenseUrl,omitempty"`
	SourceURL  string `json:"sourceUrl,omitempty"`
}

type sourceVersionResponse struct {
	ID           string `json:"id"`
	SHA256       string `json:"sha256,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	RetrievedAt  string `json:"retrievedAt,omitempty"`
}

type dataVersionResponse struct {
	Sources      []sourceVersionResponse `json:"sources"`
	GraphBuiltAt string                  `json:"graphBuiltAt,omitempty"`
}

type tripPlanResponse struct {
	Origin            locationResponse      `json:"origin"`
	Destination       locationResponse      `json:"destination"`
	RequestedDepartAt string                `json:"requestedDepartAt"`
	Itineraries       []itineraryResponse   `json:"itineraries"`
	Attributions      []attributionResponse `json:"attributions"`
	DataVersion       *dataVersionResponse  `json:"dataVersion"`
}

// --- Handler -------------------------------------------------------------

type tripPlanHandler struct {
	service *tripplan.Service
	logger  *slog.Logger
	now     func() time.Time
	timeout time.Duration
}

func (h *tripPlanHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := newRequestID()
	w.Header().Set("X-Request-Id", reqID)

	status, code, n := h.serve(w, r, reqID)

	// Coordinates, labels and error text are deliberately not logged.
	h.logger.Info("trip_plan",
		"request_id", reqID,
		"status", status,
		"code", code,
		"itineraries", n,
		"duration_ms", time.Since(start).Milliseconds(),
	)
}

// serve handles the request and returns status, error code ("" on success)
// and the number of itineraries returned, for logging.
func (h *tripPlanHandler) serve(w http.ResponseWriter, r *http.Request, reqID string) (int, string, int) {
	if !isJSON(r.Header.Get("Content-Type")) {
		return writeError(w, reqID, CodeUnsupportedMediaType, nil), CodeUnsupportedMediaType, 0
	}
	if r.ContentLength > maxTripPlanBody {
		return writeError(w, reqID, CodePayloadTooLarge, nil), CodePayloadTooLarge, 0
	}

	q, code, details := h.decode(w, r)
	if code != "" {
		return writeError(w, reqID, code, details), code, 0
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	plan, err := h.service.Plan(ctx, q)
	if err != nil {
		code, details := classify(err)
		return writeError(w, reqID, code, details), code, 0
	}

	writeJSON(w, http.StatusOK, toResponse(q, plan))
	return http.StatusOK, "", len(plan.Itineraries)
}

func isJSON(contentType string) bool {
	mt, params, err := mime.ParseMediaType(contentType)
	if err != nil || mt != "application/json" {
		return false
	}
	cs, ok := params["charset"]
	return !ok || strings.EqualFold(cs, "utf-8")
}

// decode parses and validates the body. On failure it returns an error code
// and field details.
func (h *tripPlanHandler) decode(w http.ResponseWriter, r *http.Request) (tripplan.Query, string, []errorDetail) {
	body := http.MaxBytesReader(w, r.Body, maxTripPlanBody)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()

	var req tripPlanRequest
	if err := dec.Decode(&req); err != nil {
		return tripplan.Query{}, decodeErrorCode(err), decodeErrorDetails(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return tripplan.Query{}, CodePayloadTooLarge, nil
		}
		return tripplan.Query{}, CodeInvalidInput, []errorDetail{{"body", tripplan.ReasonTrailingData}}
	}

	var probs []errorDetail
	origin := requireLocation("origin", req.Origin, &probs)
	destination := requireLocation("destination", req.Destination, &probs)

	departAt := h.now().In(wib)
	if req.DepartAt != nil {
		t, err := time.Parse(time.RFC3339, *req.DepartAt)
		if err != nil {
			probs = append(probs, errorDetail{"departAt", tripplan.ReasonInvalidFormat})
		} else {
			departAt = t
		}
	}

	maxItineraries := tripplan.DefaultMaxItineraries
	if req.MaxItineraries != nil {
		maxItineraries = *req.MaxItineraries
	}
	if len(probs) > 0 {
		return tripplan.Query{}, CodeInvalidInput, probs
	}

	q := tripplan.Query{
		Origin:         origin,
		Destination:    destination,
		DepartAt:       departAt,
		MaxItineraries: maxItineraries,
	}
	if err := q.Validate(); err != nil {
		code, details := classify(err)
		return tripplan.Query{}, code, details
	}
	return q, "", nil
}

func requireLocation(field string, l *locationRequest, probs *[]errorDetail) tripplan.Location {
	if l == nil {
		*probs = append(*probs, errorDetail{field, tripplan.ReasonRequired})
		return tripplan.Location{}
	}
	if l.Lat == nil {
		*probs = append(*probs, errorDetail{field + ".lat", tripplan.ReasonRequired})
	}
	if l.Lon == nil {
		*probs = append(*probs, errorDetail{field + ".lon", tripplan.ReasonRequired})
	}
	var loc tripplan.Location
	if l.Lat != nil && l.Lon != nil {
		loc.Coordinate = tripplan.Coordinate{Lat: *l.Lat, Lon: *l.Lon}
	}
	if l.Label != nil {
		loc.Label = *l.Label
	}
	return loc
}

func decodeErrorCode(err error) string {
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		return CodePayloadTooLarge
	}
	return CodeInvalidInput
}

func decodeErrorDetails(err error) []errorDetail {
	var (
		mbe *http.MaxBytesError
		ute *json.UnmarshalTypeError
	)
	switch {
	case errors.As(err, &mbe):
		return nil
	case errors.Is(err, io.EOF):
		return []errorDetail{{"body", tripplan.ReasonRequired}}
	case errors.As(err, &ute):
		field := ute.Field
		if field == "" {
			field = "body"
		}
		return []errorDetail{{field, tripplan.ReasonInvalidType}}
	}
	// encoding/json has no typed error for unknown fields.
	if name, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return []errorDetail{{strings.Trim(name, `"`), tripplan.ReasonUnknownField}}
	}
	return []errorDetail{{"body", tripplan.ReasonMalformedJSON}}
}

// classify maps domain errors to API codes. Error text is never forwarded.
func classify(err error) (string, []errorDetail) {
	var (
		ve  *tripplan.ValidationError
		ooc *tripplan.OutOfCoverageError
		lnr *tripplan.LocationNotRoutableError
	)
	switch {
	case errors.As(err, &ve):
		d := make([]errorDetail, len(ve.Problems))
		for i, p := range ve.Problems {
			d[i] = errorDetail{p.Field, p.Reason}
		}
		return CodeInvalidInput, d
	case errors.As(err, &ooc):
		return CodeOutOfCoverage, sideDetail(ooc.Side, "outside_service_area")
	case errors.As(err, &lnr):
		return CodeLocationNotRoutable, sideDetail(lnr.Side, "not_routable")
	case errors.Is(err, tripplan.ErrNoRoute):
		return CodeNoRoute, nil
	case errors.Is(err, tripplan.ErrRoutingTimeout), errors.Is(err, context.DeadlineExceeded):
		return CodeRoutingTimeout, nil
	case errors.Is(err, tripplan.ErrRoutingUnavailable), errors.Is(err, context.Canceled):
		return CodeRoutingUnavailable, nil
	case errors.Is(err, tripplan.ErrUpstreamInvalid):
		return CodeUpstreamInvalidResponse, nil
	}
	return CodeInternal, nil
}

func sideDetail(s tripplan.Side, reason string) []errorDetail {
	if s == tripplan.SideUnknown {
		return nil
	}
	return []errorDetail{{string(s), reason}}
}

// --- Domain → DTO --------------------------------------------------------

func toResponse(q tripplan.Query, p tripplan.Plan) tripPlanResponse {
	resp := tripPlanResponse{
		Origin:            toLocation(q.Origin),
		Destination:       toLocation(q.Destination),
		RequestedDepartAt: q.DepartAt.Format(time.RFC3339),
		Itineraries:       make([]itineraryResponse, 0, len(p.Itineraries)),
		Attributions:      make([]attributionResponse, 0, len(p.Attributions)),
	}
	for _, it := range p.Itineraries {
		ir := itineraryResponse{
			Departure:          formatTime(it.Departure),
			Arrival:            formatTime(it.Arrival),
			TimeQuality:        string(it.TimeQuality),
			DurationSeconds:    seconds(it.Duration),
			Transfers:          it.Transfers,
			WalkDistanceMeters: meters(it.WalkDistance),
			Legs:               make([]legResponse, 0, len(it.Legs)),
		}
		for _, l := range it.Legs {
			lr := legResponse{
				Type:            string(l.Type),
				Mode:            l.Mode,
				Headsign:        l.Headsign,
				From:            toPlace(l.From),
				To:              toPlace(l.To),
				DurationSeconds: seconds(l.Duration),
				DistanceMeters:  meters(l.Distance),
			}
			if l.Line != nil {
				lr.Line = &lineResponse{ID: l.Line.ID, ShortName: l.Line.ShortName, LongName: l.Line.LongName}
			}
			ir.Legs = append(ir.Legs, lr)
		}
		resp.Itineraries = append(resp.Itineraries, ir)
	}
	for _, a := range p.Attributions {
		resp.Attributions = append(resp.Attributions, attributionResponse{
			ID: a.ID, Text: a.Text, License: a.License, LicenseURL: a.LicenseURL, SourceURL: a.SourceURL,
		})
	}
	if dv := p.DataVersion; dv != nil {
		out := &dataVersionResponse{
			Sources:      make([]sourceVersionResponse, 0, len(dv.Sources)),
			GraphBuiltAt: formatOptionalTime(dv.GraphBuiltAt),
		}
		for _, s := range dv.Sources {
			out.Sources = append(out.Sources, sourceVersionResponse{
				ID:           s.ID,
				SHA256:       s.SHA256,
				LastModified: formatOptionalTime(s.LastModified),
				RetrievedAt:  formatOptionalTime(s.RetrievedAt),
			})
		}
		resp.DataVersion = out
	}
	return resp
}

func toLocation(l tripplan.Location) locationResponse {
	return locationResponse{Lat: l.Coordinate.Lat, Lon: l.Coordinate.Lon, Label: l.Label}
}

func toPlace(p tripplan.Place) placeResponse {
	return placeResponse{StopID: p.StopID, Name: p.Name, Lat: p.Coordinate.Lat, Lon: p.Coordinate.Lon}
}

func formatTime(t time.Time) string { return t.In(wib).Format(time.RFC3339) }

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func seconds(d time.Duration) int64 { return int64(d.Round(time.Second) / time.Second) }

func meters(m float64) int64 { return int64(math.Round(m)) }
