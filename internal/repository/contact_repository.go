package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/pan001/backend/internal/model"
)

// ContactFilters son los filtros opcionales de List. El puntero en nil
// significa "sin filtrar", de modo que false es un valor de filtro válido
// (por ejemplo, listar solo los terceros que no son clientes).
type ContactFilters struct {
	IsClient   *bool
	IsSupplier *bool
	IsActive   *bool
	// Search busca sin distinguir mayúsculas en name, business_name, tax_id,
	// email, phone y whatsapp. Si está vacío no filtra.
	Search string
}

// ContactRepository es el contrato de persistencia de terceros. No contiene
// reglas de negocio: solo traduce operaciones a la base de datos y los errores
// del driver a errores de dominio (por ejemplo model.ErrContactNotFound).
//
// Los terceros usan soft delete, así que las consultas y actualizaciones
// excluyen los registros con deleted_at no nulo.
type ContactRepository interface {
	// Create persiste un tercero nuevo.
	Create(ctx context.Context, contact *model.Contact) error
	// FindByID busca por identificador (los soft-deleted quedan excluidos).
	FindByID(ctx context.Context, id uuid.UUID) (*model.Contact, error)
	// List devuelve los terceros no eliminados aplicando los filtros indicados.
	List(ctx context.Context, filters ContactFilters) ([]model.Contact, error)
	// Update guarda los cambios de un tercero existente.
	Update(ctx context.Context, contact *model.Contact) error
	// Delete marca el tercero como eliminado (soft delete).
	Delete(ctx context.Context, id uuid.UUID) error
}
