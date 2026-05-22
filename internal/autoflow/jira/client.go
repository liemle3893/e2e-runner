package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/liemle3893/autoflow/internal/autoflow/state"
)

// Client is a thin wrapper around net/http bound to one set of Jira
// credentials. Zero-value is not usable; construct via NewClient.
type Client struct {
	creds *Credentials
	http  *http.Client
	base  string // https://<host>
}

// NewClient binds a client to creds. The underlying http.Client has a
// 30-second timeout and uses the default transport.
func NewClient(creds *Credentials) *Client {
	return &Client{
		creds: creds,
		http:  &http.Client{Timeout: 30 * time.Second},
		base:  "https://" + creds.Host,
	}
}

// Myself pings /rest/api/3/myself to verify the credentials are good.
// Used by `autoflow deliver doctor`. Returns the accountId on success.
func (c *Client) Myself(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/rest/api/3/myself", nil)
	if err != nil {
		return "", err
	}
	body, err := c.do(req, http.StatusOK)
	if err != nil {
		return "", err
	}
	var resp struct {
		AccountID string `json:"accountId"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse /myself: %w", err)
	}
	return resp.AccountID, nil
}

// Attachment describes one file attached to an issue.
type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	MimeType string `json:"mimeType"`
	Size     int64  `json:"size"`
	Content  string `json:"content"` // absolute download URL
}

// ListAttachments returns the attachment metadata array for a ticket.
// The ticket key is validated up-front (path-traversal guard).
func (c *Client) ListAttachments(ctx context.Context, key string) ([]Attachment, error) {
	if err := state.ValidateTicketKey(key); err != nil {
		return nil, err
	}
	path := fmt.Sprintf("/rest/api/3/issue/%s?fields=attachment", key)
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	body, err := c.do(req, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Fields struct {
			Attachment []Attachment `json:"attachment"`
		} `json:"fields"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse attachment list: %w", err)
	}
	return resp.Fields.Attachment, nil
}

// GetAttachment downloads one attachment by its absolute content URL. The
// returned reader must be closed by the caller.
func (c *Client) GetAttachment(ctx context.Context, contentURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.creds.Email, c.creds.Token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET %s: HTTP %d: %s", contentURL, resp.StatusCode, truncate(b, 200))
	}
	return resp.Body, nil
}

// newRequest builds an authenticated HTTP request relative to the Jira
// host with the standard Accept header.
func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.creds.Email, c.creds.Token)
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// do issues the request and returns the response body on a wanted status.
// Non-matching statuses produce a typed error with a truncated body so
// the user can see what went wrong without spamming logs.
func (c *Client) do(req *http.Request, wantStatus int) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != wantStatus {
		return nil, fmt.Errorf("%s %s: HTTP %d: %s",
			req.Method, req.URL.Path, resp.StatusCode, truncate(body, 400))
	}
	return body, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "…"
}
