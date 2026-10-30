// Package service contiene los casos de uso (lógica de negocio) de la
// aplicación. Depende de las entidades y de los puertos del dominio, nunca de
// Fiber ni de GORM.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

const (
	// BcryptCost es el costo de bcrypt usado para hashear contraseñas.
	BcryptCost = 12
	// maxPasswordBytes es el máximo que bcrypt procesa: lo que exceda 72 bytes
	// se ignora silenciosamente, así que lo rechazamos de forma explícita.
	maxPasswordBytes = 72
)

// TokenGenerator es el puerto de emisión de access tokens. La implementación
// concreta (JWT) vive en internal/infrastructure/auth.
type TokenGenerator interface {
	Generate(user *model.User) (string, error)
}

// RegisterInput son los datos necesarios para registrar un usuario.
type RegisterInput struct {
	Email    string
	Password string
	FullName string
	Phone    string
	Role     string
}

// AuthResult es el resultado de un registro o login exitoso.
type AuthResult struct {
	User  *model.User
	Token string
}

// AuthService agrupa los casos de uso de autenticación.
type AuthService struct {
	users  repository.UserRepository
	tokens TokenGenerator
	now    func() time.Time
}

// NewAuthService construye el servicio de autenticación.
func NewAuthService(users repository.UserRepository, tokens TokenGenerator) *AuthService {
	return &AuthService{
		users:  users,
		tokens: tokens,
		now:    func() time.Time { return time.Now().UTC() },
	}
}

// Register crea un usuario con la contraseña hasheada (bcrypt cost 12) y
// devuelve el usuario junto con un access token.
//
// Reglas:
//   - El email se normaliza (minúsculas y sin espacios) y es único.
//   - El rol por defecto es "vendedor"; si viene uno inválido, error.
//   - La contraseña nunca se guarda en texto plano.
//
// Por ahora está abierto: más adelante se restringirá a usuarios admin.
func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*AuthResult, error) {
	role := strings.TrimSpace(in.Role)
	if role == "" {
		role = model.DefaultRole
	}
	if !model.IsValidRole(role) {
		return nil, model.ErrInvalidRole
	}

	passwordHash, err := hashPassword(in.Password)
	if err != nil {
		return nil, err
	}

	user := &model.User{
		Email:        model.NormalizeEmail(in.Email),
		PasswordHash: passwordHash,
		FullName:     strings.TrimSpace(in.FullName),
		Role:         role,
		Phone:        strings.TrimSpace(in.Phone),
		IsActive:     true,
	}

	// La unicidad del email la garantiza el índice único de la base de datos,
	// que el repositorio traduce a model.ErrEmailAlreadyExists.
	if err := s.users.Create(ctx, user); err != nil {
		return nil, err
	}

	token, err := s.tokens.Generate(user)
	if err != nil {
		return nil, fmt.Errorf("generando el token de acceso: %w", err)
	}

	return &AuthResult{User: user, Token: token}, nil
}

// Login valida las credenciales, actualiza LastLoginAt y devuelve el usuario
// con un access token nuevo.
//
// Reglas:
//   - Si el email no existe o la contraseña no coincide → model.ErrInvalidCredentials.
//   - Si el usuario está desactivado → model.ErrUserInactive.
//   - Los usuarios soft-deleted no se encuentran (el scope de GORM los excluye),
//     así que reciben el mismo error que credenciales inválidas.
func (s *AuthService) Login(ctx context.Context, email, password string) (*AuthResult, error) {
	user, err := s.users.FindByEmail(ctx, model.NormalizeEmail(email))
	if err != nil {
		if errors.Is(err, model.ErrUserNotFound) {
			return nil, model.ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, model.ErrInvalidCredentials
	}

	if !user.IsActive {
		return nil, model.ErrUserInactive
	}

	loggedAt := s.now()
	if err := s.users.UpdateLastLogin(ctx, user.ID, loggedAt); err != nil {
		return nil, err
	}
	user.LastLoginAt = &loggedAt

	token, err := s.tokens.Generate(user)
	if err != nil {
		return nil, fmt.Errorf("generando el token de acceso: %w", err)
	}

	return &AuthResult{User: user, Token: token}, nil
}

// Profile devuelve los datos del usuario autenticado a partir del user_id del
// access token. Lo usa la ruta protegida GET /api/v1/auth/me.
//
// Reglas:
//   - Si el usuario no existe (o fue soft-deleted) → model.ErrUserNotFound.
//   - Si el usuario está desactivado → model.ErrUserInactive.
func (s *AuthService) Profile(ctx context.Context, userID uuid.UUID) (*model.User, error) {
	if userID == uuid.Nil {
		return nil, model.ErrUserNotFound
	}

	user, err := s.users.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !user.IsActive {
		return nil, model.ErrUserInactive
	}

	return user, nil
}

// hashPassword genera el hash bcrypt de la contraseña.
func hashPassword(password string) (string, error) {
	if len(password) > maxPasswordBytes {
		return "", model.ErrPasswordTooLong
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hasheando la contraseña: %w", err)
	}

	return string(hash), nil
}
