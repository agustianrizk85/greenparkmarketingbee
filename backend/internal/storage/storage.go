package storage

import (
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SaveFunc abstracts how a multipart file is written to disk. Gin's
// c.SaveUploadedFile matches this signature, so handlers pass it directly while
// tests can pass a stub.
type SaveFunc func(*multipart.FileHeader, string) error

// Saved describes a stored file.
type Saved struct {
	StoredName string
	Path       string
}

// Save writes an uploaded file under <uploadDir>/<subdir>/ with a collision-safe
// name derived from the document type.
func Save(uploadDir, subdir, docType string, fh *multipart.FileHeader, save SaveFunc) (*Saved, error) {
	dir := filepath.Join(uploadDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	ext := filepath.Ext(fh.Filename)
	stored := fmt.Sprintf("%s_%d%s", sanitize(docType), time.Now().UTC().UnixNano(), ext)
	dest := filepath.Join(dir, stored)
	if err := save(fh, dest); err != nil {
		return nil, err
	}
	return &Saved{StoredName: stored, Path: dest}, nil
}

func sanitize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "/", "-")
	if s == "" {
		s = "file"
	}
	return s
}
