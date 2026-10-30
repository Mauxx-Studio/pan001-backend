// Package model contiene las entidades del dominio y sus reglas, sin depender
// de HTTP ni de la base de datos.
package model

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Roles posibles de un usuario del sistema.
const (
	RoleAdmin      = "admin"
	RoleVendedor   = "vendedor"
	RolePanadero   = "panadero"
	RoleCajero     = "cajero"
	RoleSupervisor = "supervisor"

	// DefaultRole es el rol que se asigna cuando no se indica ninguno.
	DefaultRole = RoleVendedor
)

// User es la entidad de usuario. Es a la vez el modelo GORM y el DTO de dominio:
// PasswordHash lleva `json:"-"` para que nunca se serialice en una respuesta.
type User struct {
	ID              uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Email           string         `gorm:"type:varchar(255);not null;uniqueIndex" json:"email"`
	PasswordHash    string         `gorm:"type:varchar(255);not null" json:"-"`
	FullName        string         `gorm:"type:varchar(150);not null" json:"full_name"`
	Role            string         `gorm:"type:varchar(20);not null;default:'vendedor'" json:"role"`
	Phone           string         `gorm:"type:varchar(30)" json:"phone,omitempty"`
	IsActive        bool           `gorm:"not null;default:true" json:"is_active"`
	LastLoginAt     *time.Time     `json:"last_login_at,omitempty"`
	EmailVerifiedAt *time.Time     `json:"email_verified_at,omitempty"`
	DeletedAt       gorm.DeletedAt `gorm:"index" json:"-"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// TableName fija el nombre de la tabla para no depender de la convención de GORM.
func (User) TableName() string {
	return "users"
}

// BeforeCreate aplica los valores por defecto del dominio antes de insertar:
// UUID propio, email normalizado y rol por defecto.
func (u *User) BeforeCreate(_ *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	u.Email = NormalizeEmail(u.Email)
	if u.Role == "" {
		u.Role = DefaultRole
	}
	return nil
}

// NormalizeEmail unifica el formato del email para que el login sea
// insensible a mayúsculas y espacios accidentales.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// IsValidRole indica si role es uno de los roles permitidos.
func IsValidRole(role string) bool {
	switch role {
	case RoleAdmin, RoleVendedor, RolePanadero, RoleCajero, RoleSupervisor:
		return true
	default:
		return false
	}
}

// Roles devuelve la lista de roles válidos (útil para mensajes de error y docs).
func Roles() []string {
	return []string{RoleAdmin, RoleVendedor, RolePanadero, RoleCajero, RoleSupervisor}
}
