package main

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	cacheMaxEntries   = 4096
	cacheMaxRefreshes = 16
	cacheFetchTimeout = 15 * time.Second
	cacheRetryDelay   = time.Second
	cacheMaxRetryWait = time.Minute
)

var (
	ErrCacheClosed = errors.New("cache is closed")
	ErrCacheBusy   = errors.New("too many cache refreshes")
)

type Cache struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	flights map[string]*cacheFlight
	order   list.List
	bytes   int64
	closed  bool

	ttl                  time.Duration
	staleWhileRevalidate time.Duration
	staleIfError         time.Duration
	maxBytes             int64
	maxBodyBytes         int64

	client *http.Client
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// CacheResponse is shared between callers and must not be modified.
type CacheResponse struct {
	StatusCode      int
	Body            []byte
	ContentType     string
	ContentEncoding string
	ETag            string
	LastModified    string
	RetryAfter      string
}

type cacheEntry struct {
	key      string
	response *CacheResponse
	expires  time.Time
	retryAt  time.Time
	failure  *CacheResponse
	err      error
	size     int64
}

type cacheFlight struct {
	done  chan struct{}
	entry *cacheEntry
}

func NewCache(cfg ConfigCache, client *http.Client) (*Cache, error) {
	if cfg.TTL == "" {
		cfg.TTL = "5m"
	}

	ttl, err := parseCacheDuration("ttl", cfg.TTL)
	if err != nil {
		return nil, err
	}

	staleWhileRevalidate, err := parseCacheDuration("stale_while_revalidate", cfg.StaleWhileRevalidate)
	if err != nil {
		return nil, err
	}

	staleIfError, err := parseCacheDuration("stale_if_error", cfg.StaleIfError)
	if err != nil {
		return nil, err
	}

	if cfg.MaxBytes == 0 {
		cfg.MaxBytes = 64 << 20
	}

	if cfg.MaxBodyBytes == 0 {
		cfg.MaxBodyBytes = 8 << 20
	}

	if cfg.MaxBytes < 0 {
		return nil, fmt.Errorf("invalid cache max_bytes %d", cfg.MaxBytes)
	}

	if cfg.MaxBodyBytes < 0 || cfg.MaxBodyBytes > cfg.MaxBytes || cfg.MaxBodyBytes == 1<<63-1 {
		return nil, fmt.Errorf("invalid cache max_body_bytes %d", cfg.MaxBodyBytes)
	}

	if client == nil {
		client = http.DefaultClient
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Cache{
		entries:              make(map[string]*list.Element),
		flights:              make(map[string]*cacheFlight),
		ttl:                  ttl,
		staleWhileRevalidate: staleWhileRevalidate,
		staleIfError:         staleIfError,
		maxBytes:             int64(cfg.MaxBytes),
		maxBodyBytes:         int64(cfg.MaxBodyBytes),
		client:               client,
		ctx:                  ctx,
		cancel:               cancel,
	}, nil
}

func parseCacheDuration(name, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid cache %s %q: %w", name, value, err)
	}

	if duration < 0 {
		return 0, fmt.Errorf("invalid cache %s %q", name, value)
	}

	return duration, nil
}

func (c *Cache) Get(ctx context.Context, target string) (*CacheResponse, error) {
	err := ctx.Err()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()

	if c.closed {
		c.mu.Unlock()

		return nil, ErrCacheClosed
	}

	var entry *cacheEntry

	element := c.entries[target]
	if element != nil {
		entry = element.Value.(*cacheEntry)

		c.order.MoveToFront(element)
	}

	now := time.Now()

	if entry != nil && entry.response != nil && now.Before(entry.expires) {
		response := entry.response

		c.mu.Unlock()

		return response, nil
	}

	serveStale := entry != nil && entry.response != nil && now.Before(entry.expires.Add(c.staleWhileRevalidate))

	if entry != nil && now.Before(entry.retryAt) {
		c.mu.Unlock()

		if serveStale {
			return entry.response, nil
		}

		return c.result(entry, now)
	}

	flight := c.flights[target]

	if flight == nil {
		if len(c.flights) >= cacheMaxRefreshes {
			c.mu.Unlock()

			if serveStale {
				return entry.response, nil
			}

			return nil, ErrCacheBusy
		}

		flight = &cacheFlight{
			done: make(chan struct{}),
		}

		c.flights[target] = flight

		c.wg.Go(func() {
			c.refresh(target, entry, flight)
		})
	}

	c.mu.Unlock()

	if serveStale {
		return entry.response, nil
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.ctx.Done():
		return nil, ErrCacheClosed
	case <-flight.done:
		return c.result(flight.entry, time.Now())
	}
}

func (c *Cache) result(entry *cacheEntry, now time.Time) (*CacheResponse, error) {
	if entry.response != nil {
		if entry.failure == nil && entry.err == nil {
			return entry.response, nil
		}

		if now.Before(entry.expires.Add(c.staleIfError)) {
			return entry.response, nil
		}
	}

	return entry.failure, entry.err
}

func (c *Cache) refresh(target string, previous *cacheEntry, flight *cacheFlight) {
	response, err := c.fetch(target, previous)

	now := time.Now()

	entry := &cacheEntry{
		key: target,
	}

	switch {
	case err != nil || transientCacheStatus(response.StatusCode):
		entry.failure = response
		entry.err = err
		entry.retryAt = cacheRetryAt(now, response)

		if previous != nil {
			entry.response = previous.response
			entry.expires = previous.expires
		}
	case response.StatusCode == http.StatusOK:
		entry.response = response
		entry.expires = now.Add(c.ttl)
	default:
		entry.failure = response
	}

	c.mu.Lock()

	if c.closed {
		entry = &cacheEntry{err: ErrCacheClosed}
	} else {
		element := c.entries[target]
		if element != nil {
			c.remove(element)
		}

		if entry.response != nil || !entry.retryAt.IsZero() {
			c.store(entry)
		}
	}

	flight.entry = entry

	delete(c.flights, target)
	close(flight.done)

	c.mu.Unlock()
}

func (c *Cache) fetch(target string, previous *cacheEntry) (result *CacheResponse, err error) {
	ctx, cancel := context.WithTimeout(c.ctx, cacheFetchTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("create upstream request: %w", err)
	}

	request.Header.Set("Accept-Encoding", "identity")

	if previous != nil && previous.response != nil {
		if previous.response.ETag != "" {
			request.Header.Set("If-None-Match", previous.response.ETag)
		} else if previous.response.LastModified != "" {
			request.Header.Set("If-Modified-Since", previous.response.LastModified)
		}
	}

	response, err := c.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("fetch upstream: %w", err)
	}

	defer func() {
		closeErr := response.Body.Close()
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close upstream response: %w", closeErr))
		}
	}()

	if response.StatusCode == http.StatusNotModified {
		if previous == nil || previous.response == nil ||
			request.Header.Get("If-None-Match") == "" && request.Header.Get("If-Modified-Since") == "" {
			return nil, fmt.Errorf("unexpected upstream status 304")
		}

		cached := *previous.response

		etag := response.Header.Get("ETag")
		if etag != "" {
			cached.ETag = etag
		}

		modified := response.Header.Get("Last-Modified")
		if modified != "" {
			cached.LastModified = modified
		}

		return &cached, nil
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, c.maxBodyBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read upstream response: %w", err)
	}

	if int64(len(body)) > c.maxBodyBytes {
		return nil, fmt.Errorf("upstream response exceeds %d bytes", c.maxBodyBytes)
	}

	return &CacheResponse{
		StatusCode:      response.StatusCode,
		Body:            body,
		ContentType:     response.Header.Get("Content-Type"),
		ContentEncoding: response.Header.Get("Content-Encoding"),
		ETag:            response.Header.Get("ETag"),
		LastModified:    response.Header.Get("Last-Modified"),
		RetryAfter:      response.Header.Get("Retry-After"),
	}, nil
}

func (c *Cache) store(entry *cacheEntry) {
	entry.size = int64(len(entry.key)) + cacheResponseSize(entry.response) + cacheResponseSize(entry.failure)

	if entry.size > c.maxBytes {
		return
	}

	for c.bytes > c.maxBytes-entry.size || c.order.Len() >= cacheMaxEntries {
		c.remove(c.order.Back())
	}

	c.entries[entry.key] = c.order.PushFront(entry)
	c.bytes += entry.size
}

func (c *Cache) remove(element *list.Element) {
	entry := element.Value.(*cacheEntry)

	delete(c.entries, entry.key)

	c.order.Remove(element)
	c.bytes -= entry.size
}

func (c *Cache) Close() {
	c.mu.Lock()

	c.closed = true
	c.cancel()

	clear(c.entries)
	c.order.Init()

	c.bytes = 0

	c.mu.Unlock()

	c.wg.Wait()
}

func cacheResponseSize(response *CacheResponse) int64 {
	if response == nil {
		return 0
	}

	return int64(len(response.Body)) + int64(len(response.ContentType)) + int64(len(response.ContentEncoding)) + int64(len(response.ETag)) + int64(len(response.LastModified)) + int64(len(response.RetryAfter))
}

func transientCacheStatus(status int) bool {
	switch status {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func cacheRetryAt(now time.Time, response *CacheResponse) time.Time {
	if response != nil {
		value := strings.TrimSpace(response.RetryAfter)

		seconds, err := strconv.ParseInt(value, 10, 64)
		if err == nil && seconds > 0 {
			// clamp before converting to Duration to avoid overflow
			delay := time.Duration(min(seconds, int64(cacheMaxRetryWait/time.Second))) * time.Second

			return now.Add(max(cacheRetryDelay, delay))
		}

		deadline, err := http.ParseTime(value)
		if err == nil {
			delay := max(cacheRetryDelay, min(deadline.Sub(now), cacheMaxRetryWait))

			return now.Add(delay)
		}
	}

	return now.Add(cacheRetryDelay)
}
