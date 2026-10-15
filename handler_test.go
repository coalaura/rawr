package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type handlerDocumentTest struct {
	request     string
	status      int
	body        string
	contentType string
	fetches     int64
}

type handlerTransport struct {
	fetches atomic.Int64
}

func (transport *handlerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.fetches.Add(1)

	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": {"application/octet-stream"},
		},
		Body: io.NopCloser(strings.NewReader(request.URL.RequestURI())),
	}, nil
}

func TestHandleDocument(t *testing.T) {
	cfg := &Config{
		Server: ConfigServer{Port: 2112},
		Sources: []ConfigSource{
			{
				Host:           "src.example.com",
				Path:           "/<repo>/<path...>",
				Target:         "https://upstream.example.com/<repo>/<path>?raw=1",
				Index:          "index.html",
				Mime:           "text/plain",
				MimeExtensions: map[string]string{".html": "text/html"},
				Allowed: map[string][]string{
					"repo": {"project"},
					"path": {"index.html", "docs/index.html", "docs", "guide.HTML"},
				},
			},
		},
	}

	err := cfg.Init()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(cfg.Close)

	previous := config
	config = cfg

	t.Cleanup(func() {
		config = previous
	})

	transport := &handlerTransport{}
	cfg.Sources[0].cache.client = &http.Client{Transport: transport}

	tests := []handlerDocumentTest{
		{"/project/", http.StatusOK, "/project/index.html?raw=1", "text/html", 1},
		{"/project/index.html", http.StatusOK, "/project/index.html?raw=1", "text/html", 1},
		{"/project/docs/", http.StatusOK, "/project/docs/index.html?raw=1", "text/html", 2},
		{"/project/docs/index.html", http.StatusOK, "/project/docs/index.html?raw=1", "text/html", 2},
		{"/project/docs", http.StatusOK, "/project/docs?raw=1", "text/plain", 3},
		{"/project/guide.HTML?ignored=.txt", http.StatusOK, "/project/guide.HTML?raw=1", "text/html", 4},
		{"/project/private/", http.StatusNotFound, "404: Not Found", "text/plain", 4},
		{"/other/", http.StatusNotFound, "404: Not Found", "text/plain", 4},
		{"/project", http.StatusNotFound, "404: Not Found", "text/plain", 4},
	}

	for _, test := range tests {
		t.Run(test.request, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://src.example.com"+test.request, nil)

			recorder := httptest.NewRecorder()

			Handle(recorder, request)

			if recorder.Code != test.status {
				t.Errorf("status = %d, want %d", recorder.Code, test.status)
			}

			body := recorder.Body.String()
			if body != test.body {
				t.Errorf("body = %q, want %q", body, test.body)
			}

			contentType := recorder.Header().Get("Content-Type")
			if contentType != test.contentType {
				t.Errorf("Content-Type = %q, want %q", contentType, test.contentType)
			}

			fetches := transport.fetches.Load()
			if fetches != test.fetches {
				t.Errorf("upstream fetches = %d, want %d", fetches, test.fetches)
			}
		})
	}
}
