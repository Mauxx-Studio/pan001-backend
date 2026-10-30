// Package auth implementa la emisión y validación de access tokens JWT.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/pan001/backend/internal/model"
)

// DefaultTokenTTL es la vigencia por defecto del access token (24 horas).
const DefaultTokenTTL = 24 * time.Hour

// issuer identifica a la aplicación que emite los tokens.
const issuer = "pan001-api"

// Claims son los datos que viajan dentro del access token.
type Claims struct {
	// UserID es el identificador del usuario autenticado.
	UserID string `json:"user_id"`
	// Email es el email del usuario.
	Email string `json:"email"`
	// Role es el rol del usuario (admin, vendedor, panadero, cajero, supervisor).
	Role string `json:"role"`
	// RegisteredClaims aporta exp, iat, nbf, sub e iss.
	jwt.RegisteredClaims
}

// JWTManager firma y valida tokens HS256.
type JWTManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// NewJWTManager crea el gestor de tokens. Si ttl es cero o negativo se usa
// DefaultTokenTTL.
func NewJWTManager(secret string, ttl time.Duration) *JWTManager {
	if ttl <= 0 {
		ttl = DefaultTokenTTL
	}
	return &JWTManager{
		secret: []byte(secret),
		ttl:    ttl,
		now:    time.Now,
	}
}

// Generate emite un access token firmado para el usuario.
func (m *JWTManager) Generate(user *model.User) (string, error) {
	if user == nil {
		return "", errors.New("no se puede generar un token sin usuario")
	}

	now := m.now().UTC()
	claims := Claims{
		UserID: user.ID.String(),
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			Issuer:    issuer,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signed, err := token.SignedString(m.secret)
	if err != nil {
		return "", fmt.Errorf("firmando el token: %w", err)
	}

	return signed, nil
}

// Parse valida la firma y la expiración de un token y devuelve sus claims.
// Se usará desde el middleware de autenticación para proteger rutas.
func (m *JWTManager) Parse(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("método de firma inesperado: %v", token.Header["alg"])
		}
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("token inválido: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("token inválido")
	}

	return claims, nil
}
