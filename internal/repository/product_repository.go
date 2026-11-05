package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/pan001/backend/internal/model"
)

// ProductFilters son los filtros opcionales de List. El puntero en nil
// significa "sin filtrar", de modo que false es un valor de filtro válido
// (por ejemplo, listar solo los ítems que no se venden).
type ProductFilters struct {
	IsSold      *bool
	IsPurchased *bool
	IsProduced  *bool
	IsActive    *bool
	TrackStock  *bool
	// Search busca sin distinguir mayúsculas en name, sku y description.
	// Si está vacío no filtra.
	Search string
}

// ProductRepository es el contrato de persistencia del catálogo. No contiene
// reglas de negocio: solo traduce operaciones a la base de datos y los errores
// del driver a errores de dominio (por ejemplo model.ErrProductNotFound).
//
// Los productos usan soft delete, así que las consultas y actualizaciones
// excluyen los registros con deleted_at no nulo.
type ProductRepository interface {
	// Create persiste un producto nuevo.
	Create(ctx context.Context, product *model.Product) error
	// FindByID busca por identificador (los soft-deleted quedan excluidos).
	FindByID(ctx context.Context, id uuid.UUID) (*model.Product, error)
	// List devuelve los productos no eliminados aplicando los filtros indicados.
	List(ctx context.Context, filters ProductFilters) ([]model.Product, error)
	// Update guarda los cambios de un producto existente.
	Update(ctx context.Context, product *model.Product) error
	// Delete marca el producto como eliminado (soft delete).
	Delete(ctx context.Context, id uuid.UUID) error
}
