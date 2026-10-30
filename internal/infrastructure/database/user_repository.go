package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

// userRepository implementa repository.UserRepository sobre GORM.
type userRepository struct {
	db *gorm.DB
}

// NewUserRepository construye el repositorio de usuarios. Devuelve la interfaz
// para que el resto de la aplicación no dependa de GORM.
func NewUserRepository(db *gorm.DB) repository.UserRepository {
	return &userRepository{db: db}
}

// Create inserta el usuario. El email duplicado lo detecta el índice único de
// PostgreSQL, que con TranslateError: true llega como gorm.ErrDuplicatedKey.
func (r *userRepository) Create(ctx context.Context, user *model.User) error {
	err := r.db.WithContext(ctx).Create(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.ErrEmailAlreadyExists
		}
		return fmt.Errorf("creando usuario: %w", err)
	}
	return nil
}

// FindByEmail busca un usuario activo o inactivo por email. GORM aplica el
// scope del soft delete, así que los usuarios eliminados no se devuelven.
func (r *userRepository) FindByEmail(ctx context.Context, email string) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Where("email = ?", email).First(&user).Error; err != nil {
		return nil, translateUserError(err)
	}
	return &user, nil
}

// FindByID busca un usuario por su identificador.
func (r *userRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.User, error) {
	var user model.User
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&user).Error; err != nil {
		return nil, translateUserError(err)
	}
	return &user, nil
}

// UpdateLastLogin guarda la fecha del último login exitoso.
func (r *userRepository) UpdateLastLogin(ctx context.Context, id uuid.UUID, loggedAt time.Time) error {
	result := r.db.WithContext(ctx).
		Model(&model.User{}).
		Where("id = ?", id).
		Update("last_login_at", loggedAt)

	if result.Error != nil {
		return fmt.Errorf("actualizando el último login: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.ErrUserNotFound
	}
	return nil
}

// translateUserError convierte los errores de GORM en errores de dominio para
// que el service no tenga que importar gorm.
func translateUserError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ErrUserNotFound
	}
	return fmt.Errorf("consultando usuario: %w", err)
}
