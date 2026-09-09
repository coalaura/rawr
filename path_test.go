package main

import (
	"maps"
	"testing"
)

type compiledPathAllowedTest struct {
	name    string
	allowed map[string][]string
	value   string
	want    bool
}

type compilePathInvalidAllowedTest struct {
	name    string
	allowed map[string][]string
}

type compiledPathCaptureTest struct {
	name    string
	pattern string
	path    string
	value   string
	want    bool
}

type compilePathInvalidCaptureTest struct {
	name    string
	pattern string
}

type benchmarkCompiledPathAllowedTest struct {
	name string
	path string
}

func TestCompiledPathAllowed(t *testing.T) {
	t.Parallel()

	tests := []compiledPathAllowedTest{
		{"unrestricted", nil, "docs/guide.md", true},
		{"unlisted variable", map[string][]string{"repo": {"demo"}}, "archive.zip", true},
		{"nil rules", map[string][]string{"path": nil}, "guide.md", false},
		{"empty rules", map[string][]string{"path": {}}, "guide.md", false},
		{"exact match", map[string][]string{"path": {"install.sh"}}, "install.sh", true},
		{"exact mismatch", map[string][]string{"path": {"install.sh"}}, "docs/install.sh", false},
		{"prefix match", map[string][]string{"path": {"readme.*"}}, "readme.txt", true},
		{"prefix mismatch", map[string][]string{"path": {"readme.*"}}, "docs/readme.txt", false},
		{"empty wildcard remainder", map[string][]string{"path": {"readme.*"}}, "readme.", true},
		{"suffix match", map[string][]string{"path": {"*.md"}}, "guide.md", true},
		{"nested suffix match", map[string][]string{"path": {"*.md"}}, "docs/guide.md", true},
		{"suffix mismatch", map[string][]string{"path": {"*.md"}}, "guide.md.txt", false},
		{"case sensitive", map[string][]string{"path": {"*.md"}}, "README.MD", false},
		{"any rule", map[string][]string{"path": {"*.md", "readme.*", "install.sh"}}, "install.sh", true},
		{"all variables", map[string][]string{"repo": {"demo"}, "path": {"*.md"}}, "guide.md", true},
		{"repo rejected", map[string][]string{"repo": {"other"}, "path": {"*.md"}}, "guide.md", false},
		{"path rejected", map[string][]string{"repo": {"demo"}, "path": {"*.md"}}, "guide.txt", false},
		{"bare wildcard", map[string][]string{"path": {"*"}}, "docs/install.sh", true},
		{"invalid capture", map[string][]string{"path": {"*"}}, "bad space.md", false},
		{"empty capture", map[string][]string{"path": {"*"}}, "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			compiledPath, err := CompilePath("/<repo>/<path...>", test.allowed)
			if err != nil {
				t.Fatal(err)
			}

			dst := map[string]string{"old": "value"}

			got, matched := compiledPath.MatchInto("/demo/"+test.value, dst)
			if matched != test.want {
				t.Fatalf("matched = %v, want %v", matched, test.want)
			}

			want := map[string]string{"old": "value"}

			if test.want {
				want = map[string]string{
					"repo": "demo",
					"path": test.value,
				}
			}

			if !maps.Equal(got, want) {
				t.Errorf("variables = %v, want %v", got, want)
			}

			if !maps.Equal(dst, want) {
				t.Errorf("destination = %v, want %v", dst, want)
			}
		})
	}
}

func TestCompilePathInvalidAllowed(t *testing.T) {
	t.Parallel()

	tests := []compilePathInvalidAllowedTest{
		{"unknown variable", map[string][]string{"repo": {"demo"}}},
		{"empty variable", map[string][]string{"": {"demo"}}},
		{"empty rule", map[string][]string{"path": {""}}},
		{"interior wildcard", map[string][]string{"path": {"some*thing"}}},
		{"both ends", map[string][]string{"path": {"*something*"}}},
		{"repeated prefix wildcard", map[string][]string{"path": {"something**"}}},
		{"repeated suffix wildcard", map[string][]string{"path": {"**something"}}},
		{"double wildcard", map[string][]string{"path": {"**"}}},
		{"invalid rule after wildcard", map[string][]string{"path": {"*", "some*thing"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := CompilePath("/<path>", test.allowed)
			if err == nil {
				t.Fatal("expected compilation error")
			}
		})
	}
}

func TestCompiledPathCapture(t *testing.T) {
	t.Parallel()

	tests := []compiledPathCaptureTest{
		{"single segment", "/<path>", "/guide.md", "guide.md", true},
		{"single segment rejects slashes", "/<path>", "/docs/guide.md", "", false},
		{"single segment rejects empty", "/<path>", "/", "", false},
		{"single segment trailing slashes", "/<path>", "/guide.md///", "guide.md", true},
		{"single segment before literal", "/<path>/raw", "/guide.md/raw", "guide.md", true},
		{"single segment before literal rejects slashes", "/<path>/raw", "/docs/guide.md/raw", "", false},
		{"catch-all single segment", "/<path...>", "/guide.md", "guide.md", true},
		{"catch-all nested path", "/<path...>", "/docs/guide.md", "docs/guide.md", true},
		{"catch-all rejects empty", "/<path...>", "/", "", false},
		{"catch-all literal prefix", "/raw/<path...>", "/raw/docs/guide.md", "docs/guide.md", true},
		{"catch-all trailing slashes", "/<path...>", "/docs/guide.md///", "docs/guide.md", true},
		{"catch-all trailing pattern slashes", "/<path...>///", "/docs/guide.md", "docs/guide.md", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			compiledPath, err := CompilePath(test.pattern, map[string][]string{
				"path": {"*"},
			})

			if err != nil {
				t.Fatal(err)
			}

			dst := map[string]string{"old": "value"}

			got, matched := compiledPath.MatchInto(test.path, dst)
			if matched != test.want {
				t.Fatalf("matched = %v, want %v", matched, test.want)
			}

			want := map[string]string{"old": "value"}

			if test.want {
				want = map[string]string{"path": test.value}
			}

			if !maps.Equal(got, want) {
				t.Errorf("variables = %v, want %v", got, want)
			}

			if !maps.Equal(dst, want) {
				t.Errorf("destination = %v, want %v", dst, want)
			}
		})
	}
}

func TestCompilePathInvalidCapture(t *testing.T) {
	t.Parallel()

	tests := []compilePathInvalidCaptureTest{
		{"empty catch-all name", "/<...>"},
		{"invalid catch-all name", "/<bad name...>"},
		{"catch-all before variable", "/<path...>/<file>"},
		{"catch-all before literal", "/<path...>/raw"},
		{"catch-all before empty segment", "/<path...>//raw"},
		{"multiple catch-alls", "/<path...>/<other...>"},
		{"duplicate variable name", "/<path>/<path...>"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := CompilePath(test.pattern, nil)
			if err == nil {
				t.Fatal("expected compilation error")
			}
		})
	}
}

func BenchmarkCompiledPathAllowed(b *testing.B) {
	tests := []benchmarkCompiledPathAllowedTest{
		{"matched", "/demo/docs/guide.md"},
		{"rejected", "/demo/docs/guide.txt"},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			compiledPath, err := CompilePath("/<repo>/<path...>", map[string][]string{
				"repo": {"demo"},
				"path": {"*.md", "readme.*", "install.sh"},
			})

			if err != nil {
				b.Fatal(err)
			}

			dst := make(map[string]string, 2)

			b.ReportAllocs()

			for b.Loop() {
				compiledPath.MatchInto(test.path, dst)
			}
		})
	}
}
