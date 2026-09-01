package adapter_test

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liemle3893/go-tryve/internal/adapter"
)

// capturedUpload records what the server received for one multipart request.
type capturedUpload struct {
	contentType string
	fields      map[string]string
	files       map[string]uploadedFile
}

type uploadedFile struct {
	filename    string
	contentType string
	content     string
}

// uploadServer serves a single endpoint that parses a multipart body and records it.
func uploadServer(t *testing.T, got *capturedUpload) *httptest.Server {
	t.Helper()
	got.fields = map[string]string{}
	got.files = map[string]uploadedFile{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.contentType = r.Header.Get("Content-Type")

		_, params, err := mime.ParseMediaType(got.contentType)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		reader := multipart.NewReader(r.Body, params["boundary"])

		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			body, _ := io.ReadAll(part)
			if part.FileName() != "" {
				got.files[part.FormName()] = uploadedFile{
					filename:    part.FileName(),
					contentType: part.Header.Get("Content-Type"),
					content:     string(body),
				}
			} else {
				got.fields[part.FormName()] = string(body)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
}

// newConnectedHTTPAdapter returns an adapter pointed at baseURL and connected.
func newConnectedHTTPAdapter(t *testing.T, baseURL string) *adapter.HTTPAdapter {
	t.Helper()
	a := adapter.NewHTTPAdapter(baseURL)
	if err := a.Connect(context.Background()); err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	return a
}

// TestMultipartUpload covers file and text fields together — the documented
// behaviour that test suites previously had to reach for `curl -F` to get.
func TestMultipartUpload(t *testing.T) {
	dir := t.TempDir()
	csvPath := filepath.Join(dir, "members.csv")
	if err := os.WriteFile(csvPath, []byte("id,name\n1,alice\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var got capturedUpload
	srv := uploadServer(t, &got)
	defer srv.Close()

	a := newConnectedHTTPAdapter(t, srv.URL)
	res, err := a.Execute(context.Background(), "request", map[string]any{
		"method": "POST",
		"url":    "/upload",
		"multipart": []any{
			map[string]any{"name": "file", "file": csvPath, "contentType": "text/csv"},
			map[string]any{"name": "created_by", "value": "ops-admin"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status, _ := res.Data["status"].(float64); status != 201 {
		t.Errorf("expected status 201, got %v", res.Data["status"])
	}
	if !strings.HasPrefix(got.contentType, "multipart/form-data; boundary=") {
		t.Errorf("expected a multipart Content-Type with a boundary, got %q", got.contentType)
	}
	if got.fields["created_by"] != "ops-admin" {
		t.Errorf("text field not received: %#v", got.fields)
	}

	file, ok := got.files["file"]
	if !ok {
		t.Fatalf("file part not received: %#v", got.files)
	}
	if file.filename != "members.csv" {
		t.Errorf("expected the basename as filename, got %q", file.filename)
	}
	if file.contentType != "text/csv" {
		t.Errorf("expected the contentType override, got %q", file.contentType)
	}
	if file.content != "id,name\n1,alice\n" {
		t.Errorf("file content mismatch: %q", file.content)
	}
}

// TestMultipartFilenameOverride covers transmitting a different filename.
func TestMultipartFilenameOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(path, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}

	var got capturedUpload
	srv := uploadServer(t, &got)
	defer srv.Close()

	a := newConnectedHTTPAdapter(t, srv.URL)
	if _, err := a.Execute(context.Background(), "request", map[string]any{
		"method": "POST",
		"url":    "/documents",
		"multipart": []any{
			map[string]any{
				"name":     "document",
				"file":     path,
				"filename": "quarterly-report.pdf",
			},
		},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.files["document"].filename != "quarterly-report.pdf" {
		t.Errorf("expected the overridden filename, got %q", got.files["document"].filename)
	}
}

// TestMultipartRejectsConflictingSpec checks the error paths a test author is
// most likely to hit.
func TestMultipartRejectsConflictingSpec(t *testing.T) {
	var got capturedUpload
	srv := uploadServer(t, &got)
	defer srv.Close()
	a := newConnectedHTTPAdapter(t, srv.URL)

	cases := []struct {
		name   string
		params map[string]any
		want   string
	}{
		{
			name: "multipart with body",
			params: map[string]any{
				"method":    "POST",
				"url":       "/upload",
				"body":      map[string]any{"a": 1},
				"multipart": []any{map[string]any{"name": "f", "value": "v"}},
			},
			want: "mutually exclusive",
		},
		{
			name: "field with neither file nor value",
			params: map[string]any{
				"method":    "POST",
				"url":       "/upload",
				"multipart": []any{map[string]any{"name": "f"}},
			},
			want: "either",
		},
		{
			name: "missing file",
			params: map[string]any{
				"method":    "POST",
				"url":       "/upload",
				"multipart": []any{map[string]any{"name": "f", "file": "/nonexistent/nope.csv"}},
			},
			want: "could not read",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := a.Execute(context.Background(), "request", tc.params)
			if err == nil {
				t.Fatalf("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected the error to mention %q, got: %v", tc.want, err)
			}
		})
	}
}

// TestFollowRedirectsFalse covers opting out of redirect following, so a test can
// assert on the 3xx and its Location header.
func TestFollowRedirectsFalse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/go" {
			http.Redirect(w, r, "/landed", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := newConnectedHTTPAdapter(t, srv.URL)

	res, err := a.Execute(context.Background(), "request", map[string]any{
		"url":             "/go",
		"followRedirects": false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status, _ := res.Data["status"].(float64); status != 302 {
		t.Errorf("expected the redirect itself, got status %v", res.Data["status"])
	}

	// The default remains "follow".
	res, err = a.Execute(context.Background(), "request", map[string]any{"url": "/go"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status, _ := res.Data["status"].(float64); status != 200 {
		t.Errorf("expected redirects to be followed by default, got status %v", res.Data["status"])
	}
}
