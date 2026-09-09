package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
)

func Handle(w http.ResponseWriter, r *http.Request) {
	host, _, _ := strings.Cut(r.Host, ":")

	source := config.GetSourceFromHost(host)
	if source == nil {
		NotFound(w)

		log.Println(host)

		return
	}

	variables, matched := source.compiledPath.Match(r.URL.Path)
	if !matched {
		NotFound(w)

		return
	}

	target := source.Target

	for name, value := range variables {
		escaped := strings.ReplaceAll(url.PathEscape(value), "%2F", "/")

		target = strings.ReplaceAll(target, "<"+name+">", escaped)
	}

	log.Println(target)

	response, err := source.cache.Get(r.Context(), target)
	if err != nil {
		if r.Context().Err() != nil {
			return
		}

		log.Warnln(err)

		status := http.StatusBadGateway

		if errors.Is(err, ErrCacheBusy) || errors.Is(err, ErrCacheClosed) {
			status = http.StatusServiceUnavailable
		}

		http.Error(w, http.StatusText(status), status)

		return
	}

	contentType := source.Mime
	if contentType == "" {
		contentType = response.ContentType
	}

	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	if response.ContentEncoding != "" {
		w.Header().Set("Content-Encoding", response.ContentEncoding)
	}

	if response.ETag != "" {
		w.Header().Set("ETag", response.ETag)
	}

	if response.LastModified != "" {
		w.Header().Set("Last-Modified", response.LastModified)
	}

	if response.RetryAfter != "" {
		w.Header().Set("Retry-After", response.RetryAfter)
	}

	w.WriteHeader(response.StatusCode)

	_, err = w.Write(response.Body)
	if err != nil {
		log.Warnln(err)
	}
}

func NotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusNotFound)

	w.Write([]byte("404: Not Found"))
}
