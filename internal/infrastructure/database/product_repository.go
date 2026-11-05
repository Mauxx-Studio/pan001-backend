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

// productRepository implementa repository.ProductRepository sobre GORM.
type productRepository struct {
	db *gorm.DB
}

// NewProductRepository construye el repositorio de productos. Devuelve la
// interfaz para que el resto de la aplicación no dependa de GORM.
func NewProductRepository(db *gorm.DB) repository.ProductRepository {
	return &productRepository{db: db}
}

// Create inserta el producto. El UUID lo genera model.Product.BeforeCreate.
func (r *productRepository) Create(ctx context.Context, product *model.Product) error {
	if err := r.db.WithContext(ctx).Create(product).Error; err != nil {
		return fmt.Errorf("creando producto: %w", err)
	}
	return nil
}

// FindByID busca un producto por su identificador. GORM aplica el scope del
// soft delete, así que los productos eliminados no se devuelven.
func (r *productRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	var product model.Product
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&product).Error; err != nil {
		return nil, translateProductError(err)
	}
	return &product, nil
}

// List devuelve los productos no eliminados. Los filtros son opcionales y se
// combinan con AND; la búsqueda libre se compara en minúsculas para que sea
// insensible a mayúsculas sin depender de la extensión unaccent.
func (r *productRepository) List(ctx context.Context, filters repository.ProductFilters) ([]model.Product, error) {
	query := r.db.WithContext(ctx).Model(&model.Product{})

	if filters.IsSold != nil {
		query = query.Where("is_sold = ?", *filters.IsSold)
	}
	if filters.IsPurchased != nil {
		query = query.Where("is_purchased = ?", *filters.IsPurchased)
	}
	if filters.IsProduced != nil {
		query = query.Where("is_produced = ?", *filters.IsProduced)
	}
	if filters.IsActive != nil {
		query = query.Where("is_active = ?", *filters.IsActive)
	}
	if filters.TrackStock != nil {
		query = query.Where("track_stock = ?", *filters.TrackStock)
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		pattern := "%" + escapeLike(strings.ToLower(search)) + "%"
		query = query.Where(
			`(LOWER(name) LIKE ? OR LOWER(sku) LIKE ? OR LOWER(description) LIKE ?)`,
			pattern, pattern, pattern,
		)
	}

	products := make([]model.Product, 0)
	if err := query.Order("name ASC").Find(&products).Error; err != nil {
		return nil, fmt.Errorf("listando productos: %w", err)
	}
	return products, nil
}

// Update guarda todos los campos del producto. El service ya cargó la entidad y
// aplicó solo los cambios recibidos; GORM actualiza UpdatedAt automáticamente.
func (r *productRepository) Update(ctx context.Context, product *model.Product) error {
	result := r.db.WithContext(ctx).Save(product)
	if result.Error != nil {
		return fmt.Errorf("actualizando producto: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Puede pasar si el producto fue eliminado entre el FindByID y el Save.
		return model.ErrProductNotFound
	}
	return nil
}

// Delete hace un soft delete: GORM completa deleted_at y el registro deja de
// aparecer en las consultas normales.
func (r *productRepository) Delete(ctx context.Context, id uuid.UUID) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&model.Product{})
	if result.Error != nil {
		return fmt.Errorf("eliminando producto: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.ErrProductNotFound
	}
	return nil
}

// translateProductError convierte los errores de GORM en errores de dominio
// para que el service no tenga que importar gorm.
func translateProductError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ErrProductNotFound
	}
	return fmt.Errorf("consultando producto: %w", err)
}
