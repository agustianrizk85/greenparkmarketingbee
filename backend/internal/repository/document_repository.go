package repository

import (
	"errors"

	"marketingflow/internal/model"

	"gorm.io/gorm"
)

type DocumentRepository struct {
	db *gorm.DB
}

func NewDocumentRepository(db *gorm.DB) *DocumentRepository {
	return &DocumentRepository{db: db}
}

func (r *DocumentRepository) Create(d *model.Document) error {
	return r.db.Create(d).Error
}

func (r *DocumentRepository) FindByID(id uint) (*model.Document, error) {
	var d model.Document
	err := r.db.First(&d, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *DocumentRepository) Delete(id uint) error {
	return r.db.Delete(&model.Document{}, id).Error
}
