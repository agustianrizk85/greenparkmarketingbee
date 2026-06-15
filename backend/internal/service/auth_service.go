package service

import (
	"errors"

	"marketingflow/internal/dto"
	"marketingflow/internal/middleware"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid email or password")

type AuthService struct {
	users *repository.UserRepository
	tm    *middleware.TokenManager
}

func NewAuthService(users *repository.UserRepository, tm *middleware.TokenManager) *AuthService {
	return &AuthService{users: users, tm: tm}
}

func (s *AuthService) Login(req dto.LoginRequest) (*dto.LoginResponse, error) {
	user, err := s.users.FindByEmail(req.Email)
	if err != nil {
		// Avoid leaking whether the email exists.
		return nil, ErrInvalidCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		return nil, ErrInvalidCredentials
	}
	token, expiresAt, err := s.tm.Generate(user)
	if err != nil {
		return nil, err
	}
	return &dto.LoginResponse{
		Token:     token,
		ExpiresAt: expiresAt.Format("2006-01-02T15:04:05Z07:00"),
		User:      user,
	}, nil
}

// CurrentUser loads the authenticated user for the /me endpoint.
func (s *AuthService) CurrentUser(id uint) (*model.User, error) {
	return s.users.FindByID(id)
}
