package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/liemle3893/go-tryve/internal/tryve"
)

// HTTPAdapter executes HTTP requests against a target base URL.
// It maintains a persistent http.Client with cookie jar support across requests.
type HTTPAdapter struct {
	baseURL string
	client  *http.Client
	compat  tryve.CompatMode
}

// NewHTTPAdapter constructs an HTTPAdapter targeting the given baseURL.
// Connect must be called before Execute or Health.
func NewHTTPAdapter(baseURL string) *HTTPAdapter {
	return NewHTTPAdapterWithCompat(baseURL, tryve.LegacyCompat())
}

// NewHTTPAdapterWithCompat is NewHTTPAdapter with an explicit compatibility mode
// selecting the request timeout model.
func NewHTTPAdapterWithCompat(baseURL string, mode tryve.CompatMode) *HTTPAdapter {
	return &HTTPAdapter{baseURL: strings.TrimRight(baseURL, "/"), compat: mode}
}

// Name returns the adapter's registered identifier.
func (a *HTTPAdapter) Name() string { return "http" }

// Connect initialises the http.Client with a cookie jar so cookies persist
// across requests within the same adapter instance.
//
// The client carries no fixed timeout: the deadline comes from the request
// context, which the executor derives from the step's own `timeout`. A fixed
// client timeout would silently cap a step that asked for longer.
func (a *HTTPAdapter) Connect(_ context.Context) error {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return tryve.ConnectionError("http", "failed to create cookie jar", err)
	}
	a.client = &http.Client{Jar: jar}
	return nil
}

// Close releases idle connections held by the HTTP client.
func (a *HTTPAdapter) Close(_ context.Context) error {
	if a.client != nil {
		a.client.CloseIdleConnections()
	}
	return nil
}

// Health performs a lightweight HEAD request to baseURL to verify connectivity.
func (a *HTTPAdapter) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, a.baseURL, nil)
	if err != nil {
		return tryve.ConnectionError("http", "health check: failed to build request", err)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return tryve.ConnectionError("http", "health check: request failed", err)
	}
	_ = resp.Body.Close()
	return nil
}

// Execute dispatches the named action with the given parameters.
// Only the "request" action is supported; any other name returns an ADAPTER_ERROR.
func (a *HTTPAdapter) Execute(ctx context.Context, action string, params map[string]any) (*tryve.StepResult, error) {
	if action != "request" {
		return nil, tryve.AdapterError("http", action,
			fmt.Sprintf("unsupported action %q: only \"request\" is supported", action), nil)
	}
	return a.executeRequest(ctx, params)
}

// executeRequest builds and sends an HTTP request from the provided params map,
// then parses the response into a StepResult.
func (a *HTTPAdapter) executeRequest(ctx context.Context, params map[string]any) (*tryve.StepResult, error) {
	method := stringParam(params, "method", "GET")
	rawURL := stringParam(params, "url", "")
	if rawURL == "" {
		return nil, tryve.AdapterError("http", "request", "missing required param: url", nil)
	}

	// Resolve URL: absolute URLs are used directly; relative paths are prefixed
	// with baseURL.
	targetURL := rawURL
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		targetURL = a.baseURL + "/" + strings.TrimLeft(rawURL, "/")
	}

	// Append query parameters when provided.
	if q, ok := params["query"]; ok {
		if qMap, ok := q.(map[string]any); ok && len(qMap) > 0 {
			parsed, err := url.Parse(targetURL)
			if err != nil {
				return nil, tryve.AdapterError("http", "request", "invalid url", err)
			}
			qs := parsed.Query()
			for k, v := range qMap {
				qs.Set(k, fmt.Sprintf("%v", v))
			}
			parsed.RawQuery = qs.Encode()
			targetURL = parsed.String()
		}
	}

	// Build request body for methods that carry a payload.
	var bodyReader io.Reader
	hasBody := false
	multipartContentType := ""

	_, hasMultipart := params["multipart"]
	_, hasJSONBody := params["body"]
	if hasMultipart && hasJSONBody {
		return nil, tryve.AdapterError("http", "request",
			"\"multipart\" and \"body\" are mutually exclusive; use one or the other", nil)
	}

	switch {
	case hasMultipart:
		encoded, contentType, err := buildMultipartBody(params["multipart"])
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(encoded)
		multipartContentType = contentType
		hasBody = true

	case hasJSONBody && params["body"] != nil:
		upperMethod := strings.ToUpper(method)
		if upperMethod != http.MethodGet && upperMethod != http.MethodHead {
			encoded, err := json.Marshal(params["body"])
			if err != nil {
				return nil, tryve.AdapterError("http", "request", "failed to marshal body", err)
			}
			bodyReader = bytes.NewReader(encoded)
			hasBody = true
		}
	}

	// A request-level timeout bounds this call; without one the step timeout (or
	// the test timeout) applies, and failing both, defaultHTTPTimeout.
	// The legacy client capped every request at 30 seconds regardless of the step
	// or test timeout. Under the current behaviour the deadline comes from the
	// context instead, falling back to the same 30 seconds. The cap is applied
	// per request so a file pinned to an older level still gets the old model.
	reqCtx := ctx
	modernTimeouts := tryve.CompatOrDefault(ctx, a.compat).Modern(tryve.CompatAdapters)

	switch {
	case !modernTimeouts:
		var cancel context.CancelFunc
		reqCtx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	default:
		if _, hasDeadline := ctx.Deadline(); !hasDeadline {
			timeoutMs := getIntDefault(params, "timeout", defaultHTTPTimeout)
			var cancel context.CancelFunc
			reqCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutMs)*time.Millisecond)
			defer cancel()
		}
	}

	req, err := http.NewRequestWithContext(reqCtx, strings.ToUpper(method), targetURL, bodyReader)
	if err != nil {
		return nil, tryve.AdapterError("http", "request", "failed to build request", err)
	}

	// A multipart body carries its own generated boundary, so its Content-Type
	// must be set from the writer rather than guessed.
	if multipartContentType != "" {
		req.Header.Set("Content-Type", multipartContentType)
	}

	// Auto-set Content-Type when a body is present and the caller has not
	// already specified a Content-Type header.
	if hasBody && multipartContentType == "" {
		if h, ok := params["headers"]; ok {
			if hMap, ok := h.(map[string]any); ok {
				if _, has := hMap["Content-Type"]; !has {
					req.Header.Set("Content-Type", "application/json")
				}
			} else {
				req.Header.Set("Content-Type", "application/json")
			}
		} else {
			req.Header.Set("Content-Type", "application/json")
		}
	}

	// Apply caller-supplied headers. Content-Type is not overridable for a
	// multipart request: the generated boundary is part of it, and replacing the
	// header makes the body unparseable by the server.
	if h, ok := params["headers"]; ok {
		if hMap, ok := h.(map[string]any); ok {
			for k, v := range hMap {
				if multipartContentType != "" && strings.EqualFold(k, "Content-Type") {
					continue
				}
				req.Header.Set(k, fmt.Sprintf("%v", v))
			}
		}
	}

	// followRedirects: the shared client follows redirects by default; opting out
	// is per-request, so it is applied to a shallow copy of the client.
	client := a.client
	if follow, ok := params["followRedirects"].(bool); ok && !follow {
		noRedirect := *a.client
		noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
		client = &noRedirect
	}

	var resp *http.Response
	duration, err := MeasureDuration(func() error {
		var doErr error
		resp, doErr = client.Do(req)
		return doErr
	})
	if err != nil {
		return nil, tryve.AdapterError("http", "request", "request failed", err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, tryve.AdapterError("http", "request", "failed to read response body", err)
	}

	// Parse body: JSON when Content-Type indicates it, plain string otherwise.
	var parsedBody any = string(rawBody)
	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") && len(rawBody) > 0 {
		var decoded any
		if jsonErr := json.Unmarshal(rawBody, &decoded); jsonErr == nil {
			parsedBody = decoded
		}
		// On JSON parse error, parsedBody retains the raw string value.
	}

	// Collect response headers as a flat key→single-value map.
	respHeaders := make(map[string]any, len(resp.Header))
	for k := range resp.Header {
		respHeaders[k] = resp.Header.Get(k)
	}

	data := map[string]any{
		"status":     float64(resp.StatusCode),
		"statusText": resp.Status,
		"headers":    respHeaders,
		"body":       parsedBody,
		"duration":   float64(duration.Milliseconds()),
	}

	meta := map[string]any{
		"method": method,
		"url":    targetURL,
	}

	return SuccessResult(data, duration, meta), nil
}

// stringParam retrieves a string value from params by key, returning def when
// the key is absent or the value cannot be asserted to string.
func stringParam(params map[string]any, key, def string) string {
	v, ok := params[key]
	if !ok || v == nil {
		return def
	}
	s, ok := v.(string)
	if !ok {
		return def
	}
	return s
}

// defaultHTTPTimeout bounds a request when neither the step nor the test sets a
// deadline of its own.
const defaultHTTPTimeout = 30000

// buildMultipartBody encodes a multipart/form-data body from the step's
// "multipart" parameter, returning the encoded bytes and the Content-Type header
// carrying the generated boundary.
//
// Each entry names a form field and supplies either a literal "value" or a
// "file" to upload, optionally overriding the transmitted "filename" and
// "contentType".
func buildMultipartBody(spec any) ([]byte, string, error) {
	entries, ok := spec.([]any)
	if !ok {
		return nil, "", tryve.AdapterError("http", "request",
			fmt.Sprintf("\"multipart\" must be a list of fields, got %T", spec), nil)
	}
	if len(entries) == 0 {
		return nil, "", tryve.AdapterError("http", "request", "\"multipart\" must not be empty", nil)
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for i, raw := range entries {
		field, ok := raw.(map[string]any)
		if !ok {
			return nil, "", tryve.AdapterError("http", "request",
				fmt.Sprintf("multipart[%d] must be an object, got %T", i, raw), nil)
		}

		name := stringParam(field, "name", "")
		if name == "" {
			return nil, "", tryve.AdapterError("http", "request",
				fmt.Sprintf("multipart[%d] is missing the required \"name\" field", i), nil)
		}

		filePath := stringParam(field, "file", "")
		value, hasValue := field["value"]

		switch {
		case filePath != "" && hasValue:
			return nil, "", tryve.AdapterError("http", "request", fmt.Sprintf(
				"multipart field %q sets both \"file\" and \"value\"; use one or the other", name), nil)

		case filePath != "":
			if err := writeMultipartFile(writer, field, name, filePath); err != nil {
				return nil, "", err
			}

		case hasValue:
			if err := writer.WriteField(name, fmt.Sprintf("%v", value)); err != nil {
				return nil, "", tryve.AdapterError("http", "request",
					fmt.Sprintf("failed to write multipart field %q", name), err)
			}

		default:
			return nil, "", tryve.AdapterError("http", "request", fmt.Sprintf(
				"multipart field %q must set either \"file\" or \"value\"", name), nil)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, "", tryve.AdapterError("http", "request", "failed to finalise multipart body", err)
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

// writeMultipartFile appends one file part, honouring the optional filename and
// contentType overrides.
func writeMultipartFile(writer *multipart.Writer, field map[string]any, name, filePath string) error {
	contents, err := os.ReadFile(filePath)
	if err != nil {
		return tryve.AdapterError("http", "request",
			fmt.Sprintf("multipart field %q: could not read %q", name, filePath), err)
	}

	filename := stringParam(field, "filename", filepath.Base(filePath))
	contentType := stringParam(field, "contentType", "")

	headers := make(textproto.MIMEHeader)
	headers.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name=%q; filename=%q`, name, filename))
	if contentType != "" {
		headers.Set("Content-Type", contentType)
	}

	part, err := writer.CreatePart(headers)
	if err != nil {
		return tryve.AdapterError("http", "request",
			fmt.Sprintf("failed to create multipart part for %q", name), err)
	}
	if _, err := part.Write(contents); err != nil {
		return tryve.AdapterError("http", "request",
			fmt.Sprintf("failed to write multipart part for %q", name), err)
	}
	return nil
}
