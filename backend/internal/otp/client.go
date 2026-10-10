// Package otp implements tripplan.Planner on top of OpenTripPlanner 2.10's
// GTFS GraphQL API (POST {base}/otp/gtfs/v1, field planConnection).
//
// GraphQL documents and wire structs stay private to this package; callers
// only see tripplan types and tripplan errors. Upstream error text is never
// copied into returned errors.
package otp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"pandu-rute/backend/internal/tripplan"
)

const (
	// DefaultBaseURL is the local-development OTP address. OTP's own default
	// port (8080) collides with the API's, so run OTP with `--port 8082`.
	DefaultBaseURL = "http://localhost:8082"

	// DefaultTimeout bounds one upstream call. It is shorter than the HTTP
	// layer's plan timeout (8s) so a timeout can still be reported cleanly.
	DefaultTimeout = 6 * time.Second

	// DefaultMaxResponseBytes caps the upstream body read before decoding.
	DefaultMaxResponseBytes = 2 << 20

	// DefaultFeedID is the feed ID OTP assigned to the TransJakarta GTFS in
	// the local graph build (ids look like "1:G00012").
	DefaultFeedID = "1"

	// DefaultNamespace prefixes app-owned ids, e.g. "tj:stop:G00012".
	DefaultNamespace = "tj"

	graphQLPath = "/otp/gtfs/v1"
)

// Config configures a Client. Zero values select the defaults above,
// except Attributions, which are required.
type Config struct {
	BaseURL          string
	Timeout          time.Duration
	MaxResponseBytes int64
	FeedID           string
	Namespace        string
	// TimeQuality labels every itinerary. OTP 2.10 does not expose whether
	// a trip is frequency-based, so the conservative default is
	// tripplan.TimeFrequencyEstimate.
	TimeQuality  tripplan.TimeQuality
	Attributions []tripplan.Attribution
	HTTPClient   *http.Client
}

// Client is an OTP-backed tripplan.Planner.
type Client struct {
	endpoint     string
	timeout      time.Duration
	maxBytes     int64
	feedPrefix   string
	namespace    string
	timeQuality  tripplan.TimeQuality
	attributions []tripplan.Attribution
	http         *http.Client
}

var _ tripplan.Planner = (*Client)(nil)

// New validates cfg and returns a Client. It does not contact OTP, so an
// unavailable OTP never prevents the API from starting.
func New(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("otp: base URL must be an absolute http(s) URL")
	}
	if u.User != nil {
		return nil, errors.New("otp: base URL must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("otp: base URL must not contain a query or fragment")
	}
	if len(cfg.Attributions) == 0 {
		return nil, errors.New("otp: attributions are required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = DefaultMaxResponseBytes
	}
	if cfg.FeedID == "" {
		cfg.FeedID = DefaultFeedID
	}
	if cfg.Namespace == "" {
		cfg.Namespace = DefaultNamespace
	}
	if cfg.TimeQuality == "" {
		cfg.TimeQuality = tripplan.TimeFrequencyEstimate
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{}
	}
	return &Client{
		endpoint:     strings.TrimRight(u.String(), "/") + graphQLPath,
		timeout:      cfg.Timeout,
		maxBytes:     cfg.MaxResponseBytes,
		feedPrefix:   cfg.FeedID + ":",
		namespace:    cfg.Namespace,
		timeQuality:  cfg.TimeQuality,
		attributions: append([]tripplan.Attribution(nil), cfg.Attributions...),
		http:         cfg.HTTPClient,
	}, nil
}

// Endpoint returns the GraphQL URL the client posts to.
func (c *Client) Endpoint() string { return c.endpoint }

// Plan implements tripplan.Planner.
func (c *Client) Plan(ctx context.Context, q tripplan.Query) (tripplan.Plan, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	body, err := json.Marshal(graphQLRequest{Query: planQuery, Variables: newPlanVariables(q)})
	if err != nil {
		return tripplan.Plan{}, fmt.Errorf("otp: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return tripplan.Plan{}, fmt.Errorf("otp: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return tripplan.Plan{}, transportError(ctx, err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
	case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
		return tripplan.Plan{}, fmt.Errorf("%w: upstream status %d", tripplan.ErrRoutingUnavailable, resp.StatusCode)
	default:
		return tripplan.Plan{}, fmt.Errorf("%w: upstream status %d", tripplan.ErrUpstreamInvalid, resp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes+1))
	if err != nil {
		return tripplan.Plan{}, transportError(ctx, err)
	}
	if int64(len(raw)) > c.maxBytes {
		return tripplan.Plan{}, fmt.Errorf("%w: response exceeds %d bytes", tripplan.ErrUpstreamInvalid, c.maxBytes)
	}

	var gr graphQLResponse
	if err := json.Unmarshal(raw, &gr); err != nil {
		return tripplan.Plan{}, fmt.Errorf("%w: malformed JSON", tripplan.ErrUpstreamInvalid)
	}
	if len(gr.Errors) > 0 {
		return tripplan.Plan{}, fmt.Errorf("%w: %d GraphQL error(s)", tripplan.ErrUpstreamInvalid, len(gr.Errors))
	}
	if gr.Data == nil || gr.Data.PlanConnection == nil {
		return tripplan.Plan{}, fmt.Errorf("%w: missing planConnection", tripplan.ErrUpstreamInvalid)
	}
	return c.toPlan(q, gr.Data.PlanConnection)
}

// transportError classifies a failure to send the request or read the body.
func transportError(ctx context.Context, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.Canceled), errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(ctx.Err(), context.DeadlineExceeded), errors.Is(err, context.DeadlineExceeded):
		return tripplan.ErrRoutingTimeout
	}
	return fmt.Errorf("%w: transport failure", tripplan.ErrRoutingUnavailable)
}
