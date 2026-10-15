package main

import (
	"testing"

	"github.com/goccy/go-yaml"
)

type sourceDocumentTest struct {
	name    string
	pattern string
	index   string
	request string
	allowed []string
	want    string
	matched bool
}

type sourceMIMETest struct {
	name        string
	document    string
	defaultMIME string
	upstream    string
	want        string
}

type sourceInvalidTest struct {
	name       string
	index      string
	extensions map[string]string
}

func TestSourceDocument(t *testing.T) {
	t.Parallel()

	tests := []sourceDocumentTest{
		{"root", "/<path...>", "index.html", "/", nil, "index.html", true},
		{"empty catch-all", "/<repo>/<path...>", "index.html", "/project/", nil, "index.html", true},
		{"nested directory", "/<repo>/<path...>", "index.html", "/project/docs/", nil, "docs/index.html", true},
		{"repeated trailing slash", "/<repo>/<path...>", "index.html", "/project/docs///", nil, "docs/index.html", true},
		{"extensionless file", "/<repo>/<path...>", "index.html", "/project/docs", nil, "docs", true},
		{"explicit file", "/<repo>/<path...>", "index.html", "/project/docs/guide.html", nil, "docs/guide.html", true},
		{"dotted directory", "/<repo>/<path...>", "index.html", "/project/docs.v1/", nil, "docs.v1/index.html", true},
		{"missing slash", "/<repo>/<path...>", "index.html", "/project", nil, "", false},
		{"missing repo", "/<repo>/<path...>", "index.html", "/", nil, "", false},
		{"index allowed", "/<repo>/<path...>", "index.html", "/project/docs/", []string{"*.html"}, "docs/index.html", true},
		{"index denied", "/<repo>/<path...>", "index.html", "/project/docs/", []string{"*.css"}, "", false},
		{"empty rules", "/<repo>/<path...>", "index.html", "/project/", []string{}, "", false},
		{"disabled empty catch-all", "/<repo>/<path...>", "", "/project/", nil, "", false},
		{"disabled trailing slash", "/<repo>/<path...>", "", "/project/docs/", nil, "docs", true},
		{"single missing document", "/<repo>/<path>", "index.html", "/project", nil, "index.html", true},
		{"single trailing slash", "/<repo>/<path>", "index.html", "/project/", nil, "index.html", true},
		{"single repeated trailing slash", "/<repo>/<path>", "index.html", "/project///", nil, "index.html", true},
		{"single explicit index", "/<repo>/<path>", "index.html", "/project/index.html", nil, "index.html", true},
		{"single extensionless file", "/<repo>/<path>", "index.html", "/project/docs", nil, "docs", true},
		{"single nested directory", "/<repo>/<path>", "index.html", "/project/docs/", nil, "", false},
		{"single missing repo", "/<repo>/<path>", "index.html", "/", nil, "", false},
		{"single root", "/<path>", "index.html", "/", nil, "index.html", true},
		{"single empty request", "/<path>", "index.html", "", nil, "", false},
		{"single literal prefix", "/files/<repo>/<path>", "index.html", "/files/project", nil, "index.html", true},
		{"single wrong prefix", "/files/<repo>/<path>", "index.html", "/other/project", nil, "", false},
		{"single pattern trailing slash", "/<repo>/<path>/", "index.html", "/project", nil, "index.html", true},
		{"single index allowed", "/<repo>/<path>", "index.html", "/project", []string{"*.html"}, "index.html", true},
		{"single index denied", "/<repo>/<path>", "index.html", "/project", []string{"*.css"}, "", false},
		{"single disabled index", "/<repo>/<path>", "", "/project", nil, "", false},
		{"literal suffix missing", "/<repo>/index.html", "index.html", "/project", nil, "", false},
		{"literal route", "/project", "index.html", "/project", nil, "", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := ConfigSource{Path: test.pattern, Index: test.index}

			if test.allowed != nil {
				source.Allowed = map[string][]string{"path": test.allowed}
			}

			err := source.initRouting()
			if err != nil {
				t.Fatal(err)
			}

			document := source.documentPath(test.request)

			variables, matched := source.compiledPath.Match(document)
			if matched != test.matched {
				t.Fatalf("matched = %v, want %v", matched, test.matched)
			}

			if variables["path"] != test.want {
				t.Errorf("path = %q, want %q", variables["path"], test.want)
			}
		})
	}
}

func TestSourceMIME(t *testing.T) {
	t.Parallel()

	tests := []sourceMIMETest{
		{"extension override", "/docs/page.html", "text/plain", "application/octet-stream", "text/html"},
		{"case insensitive", "/docs/page.HTML", "text/plain", "", "text/html"},
		{"configured uppercase", "/assets/site.css", "text/plain", "", "text/css"},
		{"source default", "/docs/readme.md", "text/plain", "application/octet-stream", "text/plain"},
		{"upstream fallback", "/docs/readme.md", "", "text/markdown", "text/markdown"},
		{"no configuration", "/LICENSE", "", "text/plain", "text/plain"},
		{"empty override", "/empty.txt", "text/plain", "application/octet-stream", "text/plain"},
		{"no content type", "/LICENSE", "", "", ""},
		{"final extension", "/page.html.txt", "text/plain", "", "text/plain"},
		{"parent extension ignored", "/page.html/LICENSE", "text/plain", "", "text/plain"},
		{"legacy trailing slash", "/page.html/", "text/plain", "", "text/html"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := ConfigSource{
				Path: "/<path...>",
				Mime: test.defaultMIME,
				MimeExtensions: map[string]string{
					".html": "text/html",
					".CSS":  "text/css",
					".txt":  "",
				},
			}

			err := source.initRouting()
			if err != nil {
				t.Fatal(err)
			}

			got := source.contentType(test.document, test.upstream)
			if got != test.want {
				t.Errorf("Content-Type = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSourceInvalidSettings(t *testing.T) {
	t.Parallel()

	tests := []sourceInvalidTest{
		{"index directory", "docs/index.html", nil},
		{"index dot", ".", nil},
		{"index parent", "..", nil},
		{"index query", "index.html?raw=1", nil},
		{"index backslash", "docs\\index.html", nil},
		{"missing extension dot", "", map[string]string{"html": "text/html"}},
		{"empty extension", "", map[string]string{"": "text/html"}},
		{"dot extension", "", map[string]string{".": "text/html"}},
		{"compound extension", "", map[string]string{".tar.gz": "application/gzip"}},
		{"duplicate extension", "", map[string]string{".html": "text/html", ".HTML": "text/plain"}},
		{"invalid MIME", "", map[string]string{".html": "not a media type"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := ConfigSource{Path: "/<path...>", Index: test.index, MimeExtensions: test.extensions}

			err := source.initRouting()
			if err == nil {
				t.Fatal("expected configuration error")
			}
		})
	}
}

func TestSourceYAML(t *testing.T) {
	t.Parallel()

	var source ConfigSource

	err := yaml.Unmarshal([]byte("path: /<path...>\nindex: index.html\nmime: text/plain\nmime_extensions:\n  .HTML: text/html\n"), &source)
	if err != nil {
		t.Fatal(err)
	}

	err = source.initRouting()
	if err != nil {
		t.Fatal(err)
	}

	document := source.documentPath("/docs/")
	if document != "/docs/index.html" {
		t.Fatalf("document = %q", document)
	}

	contentType := source.contentType(document, "application/octet-stream")
	if contentType != "text/html" {
		t.Errorf("Content-Type = %q", contentType)
	}

	if source.Mime != "text/plain" {
		t.Errorf("default MIME = %q", source.Mime)
	}
}
