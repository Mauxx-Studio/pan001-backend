// Package repository define los puertos (interfaces) de acceso a datos. Las
// implementaciones viven en internal/infrastructure, de modo que los casos de
// uso dependen de la abstracción y no de GORM.
package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/pan001/backend/internal/model"
)

// UserRepository es el contrato de persistencia de usuarios. No contiene reglas
// de negocio: solo traduce operaciones a la base de datos y los errores del
// driver a errores de dominio (por ejemplo model.ErrUserNotFound).
type UserRepository interface {
	// Create persiste un usuario nuevo. Devuelve model.ErrEmailAlreadyExists
	// si el email ya está en uso.
	Create(ctx context.Context, user *model.User) error
	// FindByEmail busca por email (los usuarios soft-deleted quedan excluidos).
	FindByEmail(ctx context.Context, email string) (*model.User, error)
	// FindByID busca por identificador (los soft-deleted quedan excluidos).
	FindByID(ctx context.Context, id uuid.UUID) (*model.User, error)
	// UpdateLastLogin actualiza la marca de último login.
	UpdateLastLogin(ctx context.Context, id uuid.UUID, loggedAt time.Time) error
}
