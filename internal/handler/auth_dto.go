package handler

import (
	"time"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/service"
)

// registerRequest es el body de POST /api/v1/auth/register.
type registerRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72"`
	FullName string `json:"full_name" validate:"required,min=2,max=150"`
	Phone    string `json:"phone" validate:"omitempty,max=30"`
	Role     string `json:"role" validate:"omitempty,role"`
}

// loginRequest es el body de POST /api/v1/auth/login.
type loginRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,max=72"`
}

// userResponse es la representación pública del usuario: nunca incluye el hash
// de la contraseña.
type userResponse struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	FullName        string     `json:"full_name"`
	Role            string     `json:"role"`
	Phone           string     `json:"phone,omitempty"`
	IsActive        bool       `json:"is_active"`
	LastLoginAt     *time.Time `json:"last_login_at,omitempty"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// authResponse es la respuesta de register y login: usuario + access token.
type authResponse struct {
	Token     string       `json:"token"`
	TokenType string       `json:"token_type"`
	User      userResponse `json:"user"`
}

// newAuthResponse arma la respuesta a partir del resultado del servicio.
func newAuthResponse(result *service.AuthResult) authResponse {
	return authResponse{
		Token:     result.Token,
		TokenType: "Bearer",
		User:      newUserResponse(result.User),
	}
}

// newUserResponse mapea la entidad de dominio al DTO de salida.
//
// Las fechas se normalizan a UTC: PostgreSQL las devuelve en la zona horaria de
// la sesión, así que sin esto el mismo instante aparecería con offsets distintos
// según el origen del dato (memoria o base de datos).
func newUserResponse(user *model.User) userResponse {
	return userResponse{
		ID:              user.ID.String(),
		Email:           user.Email,
		FullName:        user.FullName,
		Role:            user.Role,
		Phone:           user.Phone,
		IsActive:        user.IsActive,
		LastLoginAt:     utcOrNil(user.LastLoginAt),
		EmailVerifiedAt: utcOrNil(user.EmailVerifiedAt),
		CreatedAt:       user.CreatedAt.UTC(),
		UpdatedAt:       user.UpdatedAt.UTC(),
	}
}

// utcOrNil convierte una fecha opcional a UTC respetando el nil.
func utcOrNil(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}
