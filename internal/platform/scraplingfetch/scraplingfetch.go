// Package scraplingfetch is the HTTP transport to services/scrapling-fetch, the self-hosted
// fetch service for the sources a plain client cannot reach. It is an http.RoundTripper, so a
// caller's ordinary *http.Client — its retries, size caps and status handling included — runs
// over it unchanged: each request is forwarded to the service with a mode and a proxy flag, and
// the upstream answer comes back as the response.
//
// Transport, not domain: like safehttp it knows nothing about jobs or providers.
package scraplingfetch

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Mode selects which of the service's fetchers serves a request.
type Mode string

const (
	// ModeHTTP is one request with Chrome's TLS fingerprint; any of GET, POST, PUT, DELETE.
	ModeHTTP Mode = "http"
	// ModeBrowser is a navigation in a stealth browser that solves Cloudflare challenges;
	// GET only, and the first page on a host can take minutes.
	ModeBrowser Mode = "browser"
)

// ErrUnknownMode is a Mode the service does not serve.
var ErrUnknownMode = errors.New("scraplingfetch: unknown mode")

// maxAnswerBytes bounds how much of the service's JSON answer is read. The service caps the
// upstream body at 64 MiB; base64 inflates it by a third, plus headers and framing.
const maxAnswerBytes = 90 << 20

// Config locates the service.
type Config struct {
	URL   string // base URL, e.g. http://127.0.0.1:8098
	Token string // the service's bearer token
}

// Transport forwards requests to the service. One Transport carries one mode and proxy
// choice; a provider needing another builds another.
type Transport struct {
	endpoint string
	token    string
	mode     Mode
	proxy    bool
	client   *http.Client
}

// New builds a Transport. timeout bounds one whole fetch, challenge solving included.
func New(cfg Config, mode Mode, proxy bool, timeout time.Duration) (*Transport, error) {
	if mode != ModeHTTP && mode != ModeBrowser {
		return nil, fmt.Errorf("%w %q", ErrUnknownMode, mode)
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return nil, errors.New("scraplingfetch: a token is required")
	}
	base, err := url.Parse(strings.TrimSpace(cfg.URL))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("scraplingfetch: service URL %q is not an absolute http(s) URL", cfg.URL)
	}
	return &Transport{
		endpoint: base.JoinPath("fetch").String(),
		token:    cfg.Token,
		mode:     mode,
		proxy:    proxy,
		// A plain client on purpose: the service is a fixed local endpoint, and it is the one
		// that reaches the upstream.
		client: &http.Client{Timeout: timeout},
	}, nil
}

type wireRequest struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Mode    string            `json:"mode"`
	Proxy   bool              `json:"proxy"`
}

type wireResponse struct {
	Status  int               `json:"status"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
	Error   string            `json:"error,omitempty"`
}

// RoundTrip sends req through the service. An upstream status of any kind is a response; a
// service that cannot be reached, fails the fetch, or answers malformed is an error — never a
// silent fallback to a direct request.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil {
		b, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("scraplingfetch: read request body: %w", err)
		}
		body = b
	}
	headers := make(map[string]string, len(req.Header))
	for k, vs := range req.Header {
		headers[k] = strings.Join(vs, ", ")
	}
	payload, err := json.Marshal(wireRequest{
		URL:     req.URL.String(),
		Method:  req.Method,
		Headers: headers,
		Body:    base64.StdEncoding.EncodeToString(body),
		Mode:    string(t.mode),
		Proxy:   t.proxy,
	})
	if err != nil {
		return nil, fmt.Errorf("scraplingfetch: encode request: %w", err)
	}
	answer, err := t.post(req.Context(), payload)
	if err != nil {
		return nil, fmt.Errorf("scraplingfetch: %s %s: %w", req.Method, req.URL, err)
	}
	upstream, err := base64.StdEncoding.DecodeString(answer.Body)
	if err != nil {
		return nil, fmt.Errorf("scraplingfetch: %s %s: body is not base64: %w", req.Method, req.URL, err)
	}

	final := req.Clone(req.Context())
	if answer.URL != "" {
		if u, err := url.Parse(answer.URL); err == nil {
			final.URL = u
		}
	}
	header := make(http.Header, len(answer.Headers))
	for k, v := range answer.Headers {
		header.Set(k, v)
	}
	return &http.Response{
		Status:        strconv.Itoa(answer.Status) + " " + http.StatusText(answer.Status),
		StatusCode:    answer.Status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(upstream)),
		ContentLength: int64(len(upstream)),
		Request:       final,
	}, nil
}

// ServiceError reports a non-200 response from the fetch service itself.
type ServiceError struct {
	StatusCode int
	message    string
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("service answered %d: %s", e.StatusCode, e.message)
}

func (t *Transport) post(ctx context.Context, payload []byte) (wireResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(payload))
	if err != nil {
		return wireResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+t.token)
	resp, err := t.client.Do(httpReq)
	if err != nil {
		return wireResponse{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswerBytes))
	if err != nil {
		return wireResponse{}, fmt.Errorf("read answer: %w", err)
	}
	var answer wireResponse
	decodeErr := json.Unmarshal(raw, &answer)
	if resp.StatusCode != http.StatusOK {
		msg := answer.Error
		if decodeErr != nil || msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return wireResponse{}, &ServiceError{StatusCode: resp.StatusCode, message: msg}
	}
	if decodeErr != nil {
		return wireResponse{}, fmt.Errorf("decode answer: %w", decodeErr)
	}
	if answer.Status < 100 || answer.Status > 999 {
		return wireResponse{}, fmt.Errorf("answer carries no upstream status")
	}
	return answer, nil
}
