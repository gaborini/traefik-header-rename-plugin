package traefik_custom_headers_plugin

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// Rename holds one rename configuration.
type renameData struct {
	ExistingHeaderName string `json:"existingHeaderName"`
	NewHeaderName      string `json:"newHeaderName"`
}

// Config holds the plugin configuration.
type Config struct {
	RenameData []renameData `json:"renameData"`
}

// CreateConfig creates and initializes the plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// renameHeaders is the main plugin structure.
type renameHeaders struct {
	name    string
	next    http.Handler
	renames []renameData
}

// New creates a new Custom Header plugin.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	// Config validation
	if config == nil {
		return nil, errors.New("config cannot be nil")
	}
	
	if len(config.RenameData) == 0 {
		return nil, errors.New("no rename data configured: at least one rename rule is required")
	}
	
	// Validate each rename configuration
	for i, rename := range config.RenameData {
		if rename.ExistingHeaderName == "" {
			return nil, fmt.Errorf("rename rule %d: existing header name cannot be empty", i)
		}
		if rename.NewHeaderName == "" {
			return nil, fmt.Errorf("rename rule %d: new header name cannot be empty", i)
		}
	}
	
	return &renameHeaders{
		name:    name,
		next:    next,
		renames: config.RenameData,
	}, nil
}

// ServeHTTP implements the http.Handler interface.
func (r *renameHeaders) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	wrappedWriter := &responseWriter{
		ResponseWriter:  rw,
		headersToRename: r.renames,
	}
	
	r.next.ServeHTTP(wrappedWriter, req)
}

// responseWriter wraps the original http.ResponseWriter to intercept and modify headers.
type responseWriter struct {
	http.ResponseWriter
	headersToRename []renameData
	headerWritten   bool
}

// WriteHeader intercepts the status code writing to rename headers before they are sent.
func (r *responseWriter) WriteHeader(statusCode int) {
	if r.headerWritten {
		return
	}
	
	// Rename headers before writing
	for _, headerToRename := range r.headersToRename {
		headerValues := r.Header().Values(headerToRename.ExistingHeaderName)
		
		if len(headerValues) == 0 {
			continue
		}
		
		// Remove old header and add with new name
		r.Header().Del(headerToRename.ExistingHeaderName)
		r.Header()[headerToRename.NewHeaderName] = headerValues
	}
	
	r.headerWritten = true
	r.ResponseWriter.WriteHeader(statusCode)
}

// Write ensures headers are written before body.
func (r *responseWriter) Write(bytes []byte) (int, error) {
	if !r.headerWritten {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(bytes)
}

// Hijack implements the http.Hijacker interface for WebSocket support.
func (r *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := r.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("ResponseWriter of type %T does not support hijacking", r.ResponseWriter)
	}
	return hijacker.Hijack()
}

// Flush implements the http.Flusher interface for SSE and streaming responses.
func (r *responseWriter) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Push implements the http.Pusher interface for HTTP/2 server push support.
func (r *responseWriter) Push(target string, opts *http.PushOptions) error {
	if pusher, ok := r.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, opts)
	}
	return http.ErrNotSupported
}
