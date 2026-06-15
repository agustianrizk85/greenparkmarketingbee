package middleware

import (
	"errors"
	"time"

	"marketingflow/internal/model"

	"github.com/golang-jwt/jwt/v5"
)

// TokenManager issues and verifies JWT access tokens.
type TokenManager struct {
	secret []byte
	expiry time.Duration
}

func NewTokenManager(secret string, expiryHours int) *TokenManager {
	return &TokenManager{
		secret: []byte(secret),
		expiry: time.Duration(expiryHours) * time.Hour,
	}
}

// Claims is the JWT payload carried for an authenticated user.
type Claims struct {
	UserID uint       `json:"uid"`
	Email  string     `json:"email"`
	Role   model.Role `json:"role"`
	jwt.RegisteredClaims
}

func (m *TokenManager) Generate(u *model.User) (string, time.Time, error) {
	expiresAt := nowUTC().Add(m.expiry)
	claims := Claims{
		UserID: u.ID,
		Email:  u.Email,
		Role:   u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(nowUTC()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.secret)
	return signed, expiresAt, err
}

func (m *TokenManager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

func nowUTC() time.Time { return time.Now().UTC() }
