package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/pan001/backend/internal/model"
)

// OrderFilters son los filtros opcionales de List. Los punteros en nil y las
// cadenas vacías significan "sin filtrar".
type OrderFilters struct {
	// ContactID limita el listado a los pedidos de un tercero.
	ContactID *uuid.UUID
	// Status filtra por estado del pedido.
	Status string
	// PaymentStatus filtra por estado de pago.
	PaymentStatus string
	// From y To acotan ordered_at (ambos inclusive). Nil = sin límite.
	From *time.Time
	To   *time.Time
	// Search busca sin distinguir mayúsculas en code, notes y en el nombre del
	// tercero. Si está vacío no filtra.
	Search string
}

// OrderRepository es el contrato de persistencia de pedidos (cabecera y
// líneas). No contiene reglas de negocio: solo traduce operaciones a la base de
// datos y los errores del driver a errores de dominio.
//
// Los pedidos usan soft delete, así que las consultas excluyen los registros
// con deleted_at no nulo. Las líneas también usan soft delete y solo se leen
// junto con el detalle del pedido.
//
// Los métodos de líneas y NextDailySequence se usan dentro de WithTransaction
// para que cabecera e ítems se confirmen juntos.
type OrderRepository interface {
	// Create persiste un pedido nuevo junto con sus líneas (order.Items).
	// Devuelve model.ErrOrderCodeConflict si el código choca con el índice
	// único (carrera entre dos peticiones simultáneas).
	Create(ctx context.Context, order *model.Order) error
	// FindByID busca por identificador y precarga las líneas no eliminadas.
	FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error)
	// List devuelve los pedidos no eliminados aplicando los filtros indicados,
	// del más reciente al más antiguo. No precarga las líneas.
	List(ctx context.Context, filters OrderFilters) ([]model.Order, error)
	// Update guarda los cambios de la cabecera (no toca las líneas).
	Update(ctx context.Context, order *model.Order) error
	// Delete marca el pedido y sus líneas como eliminados (soft delete).
	Delete(ctx context.Context, id uuid.UUID) error

	// NextDailySequence devuelve el próximo número de secuencia diaria libre
	// para el código del pedido. Cuenta también los pedidos soft-deleted,
	// porque su código sigue ocupando el índice único.
	NextDailySequence(ctx context.Context, day time.Time) (int64, error)

	// CreateItems inserta líneas nuevas de un pedido existente.
	CreateItems(ctx context.Context, items []model.OrderItem) error
	// UpdateItem guarda los cambios de una línea existente.
	UpdateItem(ctx context.Context, item *model.OrderItem) error
	// SoftDeleteItems marca las líneas indicadas como eliminadas. No se borran
	// físicamente: quedan para el histórico de entregas.
	SoftDeleteItems(ctx context.Context, orderID uuid.UUID, ids []uuid.UUID) error

	// WithTransaction ejecuta fn dentro de una transacción. El repositorio que
	// recibe fn comparte la transacción, de modo que un error deshace todo.
	WithTransaction(ctx context.Context, fn func(tx OrderRepository) error) error
}
