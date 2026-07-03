package service

import (
	"fmt"
	"mime/multipart"
	"os"

	"marketingflow/internal/model"
	"marketingflow/internal/repository"
	"marketingflow/internal/storage"
)

type DocumentService struct {
	docs      *repository.DocumentRepository
	uploadDir string
}

func NewDocumentService(docs *repository.DocumentRepository, uploadDir string) *DocumentService {
	return &DocumentService{docs: docs, uploadDir: uploadDir}
}

// UploadParams describes a single file upload.
type UploadParams struct {
	WorkItemID uint
	WorkStepID *uint
	DocType    string
	UploadedBy uint
}

// Upload stores the file on disk and records its metadata.
func (s *DocumentService) Upload(fh *multipart.FileHeader, p UploadParams, save storage.SaveFunc) (*model.Document, error) {
	subdir := fmt.Sprintf("work_%d", p.WorkItemID)
	saved, err := storage.Save(s.uploadDir, subdir, p.DocType, fh, save)
	if err != nil {
		return nil, err
	}

	doc := &model.Document{
		WorkItemID:   p.WorkItemID,
		WorkStepID:   p.WorkStepID,
		DocType:      p.DocType,
		OriginalName: fh.Filename,
		StoredName:   saved.StoredName,
		Path:         saved.Path,
		MimeType:     fh.Header.Get("Content-Type"),
		SizeBytes:    fh.Size,
		UploadedBy:   p.UploadedBy,
	}
	if err := s.docs.Create(doc); err != nil {
		_ = os.Remove(saved.Path)
		return nil, err
	}
	return doc, nil
}

func (s *DocumentService) Get(id uint) (*model.Document, error) {
	return s.docs.FindByID(id)
}

// PurgeAll removes every stored upload (the per-work-item subdirectories),
// leaving the upload root in place. Called by the "delete all data" reset after
// the document rows are gone, so no orphan files linger.
func (s *DocumentService) PurgeAll() error {
	entries, err := os.ReadDir(s.uploadDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.Name() == ".gitkeep" {
			continue
		}
		if err := os.RemoveAll(s.uploadDir + "/" + e.Name()); err != nil {
			return err
		}
	}
	return nil
}
