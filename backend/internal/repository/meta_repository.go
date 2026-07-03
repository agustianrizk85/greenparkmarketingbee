package repository

import (
	"errors"

	"marketingflow/internal/model"

	"gorm.io/gorm"
)

// MetaRepository persists the Meta OAuth app config (singleton) and the set of
// connected accounts (one row per OAuth login).
type MetaRepository struct {
	db *gorm.DB
}

func NewMetaRepository(db *gorm.DB) *MetaRepository {
	return &MetaRepository{db: db}
}

const metaAppConfigID = 1

// AppConfig returns the singleton OAuth app config, creating an empty row on
// first access so callers always get a non-nil record.
func (r *MetaRepository) AppConfig() (*model.MetaAppConfig, error) {
	var cfg model.MetaAppConfig
	err := r.db.First(&cfg, metaAppConfigID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cfg = model.MetaAppConfig{ID: metaAppConfigID}
		if err := r.db.Create(&cfg).Error; err != nil {
			return nil, err
		}
		return &cfg, nil
	}
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// SaveAppConfig upserts the singleton config row.
func (r *MetaRepository) SaveAppConfig(cfg *model.MetaAppConfig) error {
	cfg.ID = metaAppConfigID
	return r.db.Save(cfg).Error
}

// ListConnections returns every connected account, active first then newest.
func (r *MetaRepository) ListConnections() ([]model.MetaConnection, error) {
	var out []model.MetaConnection
	err := r.db.Order("is_active DESC, created_at DESC").Find(&out).Error
	return out, err
}

// FindActiveConnection returns the active connection, or nil when none is set.
func (r *MetaRepository) FindActiveConnection() (*model.MetaConnection, error) {
	var c model.MetaConnection
	err := r.db.Where("is_active = ?", true).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *MetaRepository) FindConnection(id uint) (*model.MetaConnection, error) {
	var c model.MetaConnection
	err := r.db.First(&c, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *MetaRepository) FindConnectionByMetaUserID(metaUserID string) (*model.MetaConnection, error) {
	var c model.MetaConnection
	err := r.db.Where("meta_user_id = ?", metaUserID).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CountConnections reports how many accounts are connected.
func (r *MetaRepository) CountConnections() (int64, error) {
	var n int64
	err := r.db.Model(&model.MetaConnection{}).Count(&n).Error
	return n, err
}

func (r *MetaRepository) CreateConnection(c *model.MetaConnection) error {
	return r.db.Create(c).Error
}

func (r *MetaRepository) SaveConnection(c *model.MetaConnection) error {
	return r.db.Save(c).Error
}

// SetActive marks one connection active and clears the flag on all others, in a
// single transaction so there is always at most one active account.
func (r *MetaRepository) SetActive(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.MetaConnection{}).Where("is_active = ?", true).Update("is_active", false).Error; err != nil {
			return err
		}
		res := tx.Model(&model.MetaConnection{}).Where("id = ?", id).Update("is_active", true)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// DeleteConnection removes a connection. When it was the active one, the most
// recent remaining connection is promoted so the proxy keeps working.
func (r *MetaRepository) DeleteConnection(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var c model.MetaConnection
		if err := tx.First(&c, id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if err := tx.Delete(&model.MetaConnection{}, id).Error; err != nil {
			return err
		}
		if c.IsActive {
			var next model.MetaConnection
			if err := tx.Order("created_at DESC").First(&next).Error; err == nil {
				return tx.Model(&model.MetaConnection{}).Where("id = ?", next.ID).Update("is_active", true).Error
			}
		}
		return nil
	})
}
