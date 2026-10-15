package main

import (
	"fmt"
	"mime"
	"path"
	"strings"
)

func (s *ConfigSource) initRouting() error {
	if s.Index != "" {
		if s.Index == "." || s.Index == ".." || strings.Contains(s.Index, "/") || !validPathChars(s.Index) {
			return fmt.Errorf("invalid index filename %q", s.Index)
		}
	}

	compiledPath, err := CompilePath(s.Path, s.Allowed)
	if err != nil {
		return err
	}

	extensions := make(map[string]string, len(s.MimeExtensions))

	for extension, contentType := range s.MimeExtensions {
		if len(extension) < 2 || path.Ext(extension) != extension || strings.Contains(extension, "/") || !validPathChars(extension) {
			return fmt.Errorf("invalid MIME extension %q", extension)
		}

		normalized := strings.ToLower(extension)
		if _, exists := extensions[normalized]; exists {
			return fmt.Errorf("duplicate MIME extension %q", extension)
		}

		if contentType != "" {
			_, _, err = mime.ParseMediaType(contentType)
			if err != nil {
				return fmt.Errorf("invalid MIME type for extension %q: %w", extension, err)
			}
		}

		extensions[normalized] = contentType
	}

	s.compiledPath = compiledPath
	s.mimeExtensions = extensions
	s.indexParentSlashes = -1

	lastPart := compiledPath.parts[len(compiledPath.parts)-1]
	if s.Index != "" && lastPart.name != "" && !lastPart.catchAll {
		// Only the final single-segment variable may be implicitly filled by the index.
		s.indexParentSlashes = strings.Count(trimTrailingSlashes(s.Path), "/") - 1
	}

	return nil
}

func (s *ConfigSource) documentPath(requestPath string) string {
	if s.Index == "" {
		return requestPath
	}

	if strings.HasSuffix(requestPath, "/") {
		return strings.TrimRight(requestPath, "/") + "/" + s.Index
	}

	if s.indexParentSlashes >= 0 && strings.HasPrefix(requestPath, "/") && strings.Count(requestPath, "/") == s.indexParentSlashes {
		return requestPath + "/" + s.Index
	}

	return requestPath
}

func (s *ConfigSource) contentType(documentPath, upstream string) string {
	if len(s.mimeExtensions) != 0 {
		extension := path.Ext(trimTrailingSlashes(documentPath))

		contentType := s.mimeExtensions[strings.ToLower(extension)]
		if contentType != "" {
			return contentType
		}
	}

	if s.Mime != "" {
		return s.Mime
	}

	return upstream
}
