// Package media provides media storage (stub for voicebot).
package media

import (
	"context"
	"io"
)

// MediaStore stores media files.
type MediaStore interface {
	Store(ctx context.Context, scope string, filename string, contentType string, data []byte) (ref string, err error)
	Resolve(ref string) ([]byte, error)
	ResolveWithMeta(ref string) ([]byte, Meta, error)
	ReleaseAll(scope string) error
}

// Meta contains media metadata.
type Meta struct {
	Filename    string
	ContentType string
	Size        int64
}

// NopMediaStore is a no-op media store.
type NopMediaStore struct{}

// Store stores media (no-op).
func (s *NopMediaStore) Store(ctx context.Context, scope string, filename string, contentType string, data []byte) (string, error) {
	return "", nil
}

// Resolve resolves media (no-op).
func (s *NopMediaStore) Resolve(ref string) ([]byte, error) {
	return nil, nil
}

// ResolveWithMeta resolves media with metadata (no-op).
func (s *NopMediaStore) ResolveWithMeta(ref string) ([]byte, Meta, error) {
	return nil, Meta{}, nil
}

// ReleaseAll releases all media in a scope (no-op).
func (s *NopMediaStore) ReleaseAll(scope string) error {
	return nil
}

// NewNopMediaStore creates a new no-op media store.
func NewNopMediaStore() *NopMediaStore {
	return &NopMediaStore{}
}

// MediaReader reads media from a reader.
type MediaReader struct {
	r io.Reader
}

// NewMediaReader creates a new media reader.
func NewMediaReader(r io.Reader) *MediaReader {
	return &MediaReader{r: r}
}

// Read reads from the media reader.
func (r *MediaReader) Read(p []byte) (n int, err error) {
	return r.r.Read(p)
}
