package repository

import (
	"errors"

	"marketingflow/internal/model"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("record not found")

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) FindByEmail(email string) (*model.User, error) {
	var u model.User
	err := r.db.Where("email = ?", email).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) FindByID(id uint) (*model.User, error) {
	var u model.User
	err := r.db.First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) Create(u *model.User) error {
	return r.db.Create(u).Error
}

// List mengembalikan seluruh akun, diurutkan supaya pemilih tujuan Pre-Brief
// tampil stabil: kepala departemen dulu, lalu menurut nama.
func (r *UserRepository) List() ([]model.User, error) {
	var out []model.User
	err := r.db.Order("CASE WHEN role = 'kadep' THEN 0 ELSE 1 END, name").Find(&out).Error
	return out, err
}

func (r *UserRepository) Count() (int64, error) {
	var n int64
	err := r.db.Model(&model.User{}).Count(&n).Error
	return n, err
}
