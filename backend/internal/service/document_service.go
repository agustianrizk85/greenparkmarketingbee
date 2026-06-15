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
