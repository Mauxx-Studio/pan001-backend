package database

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

// contactRepository implementa repository.ContactRepository sobre GORM.
type contactRepository struct {
	db *gorm.DB
}

// NewContactRepository construye el repositorio de terceros. Devuelve la
// interfaz para que el resto de la aplicación no dependa de GORM.
func NewContactRepository(db *gorm.DB) repository.ContactRepository {
	return &contactRepository{db: db}
}

// Create inserta el tercero. El UUID lo genera model.Contact.BeforeCreate.
func (r *contactRepository) Create(ctx context.Context, contact *model.Contact) error {
	if err := r.db.WithContext(ctx).Create(contact).Error; err != nil {
		return fmt.Errorf("creando tercero: %w", err)
	}
	return nil
}

// FindByID busca un tercero por su identificador. GORM aplica el scope del
// soft delete, así que los terceros eliminados no se devuelven.
func (r *contactRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Contact, error) {
	var contact model.Contact
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&contact).Error; err != nil {
		return nil, translateContactError(err)
	}
	return &contact, nil
}

// List devuelve los terceros no eliminados. Los filtros son opcionales y se
// combinan con AND; la búsqueda libre se compara en minúsculas para que sea
// insensible a mayúsculas sin depender de la extensión unaccent.
func (r *contactRepository) List(ctx context.Context, filters repository.ContactFilters) ([]model.Contact, error) {
	query := r.db.WithContext(ctx).Model(&model.Contact{})

	if filters.IsClient != nil {
		query = query.Where("is_client = ?", *filters.IsClient)
	}
	if filters.IsSupplier != nil {
		query = query.Where("is_supplier = ?", *filters.IsSupplier)
	}
	if filters.IsActive != nil {
		query = query.Where("is_active = ?", *filters.IsActive)
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		pattern := "%" + escapeLike(strings.ToLower(search)) + "%"
		query = query.Where(
			`(LOWER(name) LIKE ? OR LOWER(business_name) LIKE ? OR LOWER(tax_id) LIKE ?`+
				` OR LOWER(email) LIKE ? OR LOWER(phone) LIKE ? OR LOWER(whatsapp) LIKE ?)`,
			pattern, pattern, pattern, pattern, pattern, pattern,
		)
	}

	contacts := make([]model.Contact, 0)
	if err := query.Order("name ASC").Find(&contacts).Error; err != nil {
		return nil, fmt.Errorf("listando terceros: %w", err)
	}
	return contacts, nil
}

// Update guarda todos los campos del tercero. El service ya cargó la entidad y
// aplicó solo los cambios recibidos; GORM actualiza UpdatedAt automáticamente.
func (r *contactRepository) Update(ctx context.Context, contact *model.Contact) error {
	result := r.db.WithContext(ctx).Save(contact)
	if result.Error != nil {
		return fmt.Errorf("actualizando tercero: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Puede pasar si el tercero fue eliminado entre el FindByID y el Save.
		return model.ErrContactNotFound
	}
	return nil
}

// Delete hace un soft delete: GORM completa deleted_at y el registro deja de
// aparecer en las consultas normales.
func (r *contactRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Contact{})
	if result.Error != nil {
		return fmt.Errorf("eliminando tercero: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.ErrContactNotFound
	}
	return nil
}

// translateContactError convierte los errores de GORM en errores de dominio
// para que el service no tenga que importar gorm.
func translateContactError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ErrContactNotFound
	}
	return fmt.Errorf("consultando tercero: %w", err)
}

// escapeLike neutraliza los comodines de LIKE que pueda escribir el usuario, de
// modo que search=100% busque ese texto literal y no "todo". En PostgreSQL la
// barra invertida es el carácter de escape por defecto de LIKE.
func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}
