package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

// orderRepository implementa repository.OrderRepository sobre GORM.
type orderRepository struct {
	db *gorm.DB
}

// NewOrderRepository construye el repositorio de pedidos. Devuelve la interfaz
// para que el resto de la aplicación no dependa de GORM.
func NewOrderRepository(db *gorm.DB) repository.OrderRepository {
	return &orderRepository{db: db}
}

// Create inserta la cabecera y, si vienen cargadas, sus líneas. El UUID lo
// genera BeforeCreate y GORM completa order_id en cada ítem.
//
// Un choque con el índice único de code se traduce a model.ErrOrderCodeConflict
// para que el service reintente con la secuencia siguiente: es el único
// duplicado posible en este INSERT (los UUID son aleatorios).
func (r *orderRepository) Create(ctx context.Context, order *model.Order) error {
	if err := r.db.WithContext(ctx).Create(order).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return model.ErrOrderCodeConflict
		}
		return fmt.Errorf("creando pedido: %w", err)
	}
	return nil
}

// FindByID busca un pedido por su identificador y precarga sus líneas en orden
// de creación. GORM aplica el scope del soft delete tanto en la cabecera como
// en el Preload, así que ni los pedidos ni las líneas eliminadas se devuelven.
func (r *orderRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	var order model.Order

	err := r.db.WithContext(ctx).
		Preload("Items", func(db *gorm.DB) *gorm.DB {
			return db.Order("order_items.created_at ASC, order_items.id ASC")
		}).
		Where("id = ?", id).
		First(&order).Error
	if err != nil {
		return nil, translateOrderError(err)
	}

	if order.Items == nil {
		order.Items = []model.OrderItem{}
	}
	return &order, nil
}

// List devuelve los pedidos no eliminados, del más reciente al más antiguo. No
// precarga las líneas: el listado es un resumen.
//
// La búsqueda libre necesita el nombre del tercero, así que cuando hay `search`
// se agrega un LEFT JOIN con contacts y se selecciona solo orders.* para no
// mezclar columnas homónimas (por ejemplo deleted_at de ambas tablas).
func (r *orderRepository) List(ctx context.Context, filters repository.OrderFilters) ([]model.Order, error) {
	query := r.db.WithContext(ctx).Model(&model.Order{})

	if filters.ContactID != nil {
		query = query.Where("orders.contact_id = ?", *filters.ContactID)
	}
	if filters.Status != "" {
		query = query.Where("orders.status = ?", filters.Status)
	}
	if filters.PaymentStatus != "" {
		query = query.Where("orders.payment_status = ?", filters.PaymentStatus)
	}
	if filters.From != nil {
		query = query.Where("orders.ordered_at >= ?", *filters.From)
	}
	if filters.To != nil {
		query = query.Where("orders.ordered_at <= ?", *filters.To)
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		pattern := "%" + escapeLike(strings.ToLower(search)) + "%"
		query = query.
			Select("orders.*").
			Joins("LEFT JOIN contacts ON contacts.id = orders.contact_id").
			Where(
				`(LOWER(orders.code) LIKE ? OR LOWER(COALESCE(orders.notes, '')) LIKE ? OR LOWER(contacts.name) LIKE ?)`,
				pattern, pattern, pattern,
			)
	}

	orders := make([]model.Order, 0)
	if err := query.Order("orders.ordered_at DESC, orders.created_at DESC").Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("listando pedidos: %w", err)
	}
	return orders, nil
}

// orderUpdateColumns son las columnas que el PUT puede reescribir en la
// cabecera. Se listan explícitamente (en vez de usar Save) para no tocar code,
// created_at ni las asociaciones, y para que GORM incluya también los valores
// cero (por ejemplo amount_paid=0 o delivery_at=NULL).
var orderUpdateColumns = []string{
	"contact_id", "status", "delivery_at", "delivery_type", "subtotal",
	"discount_amount", "final_amount", "payment_method", "payment_status",
	"amount_paid", "parent_order_id", "notes", "is_active", "updated_at",
}

// Update guarda los cambios de la cabecera sin tocar las líneas. updated_at va
// en la lista de columnas porque, con un Select explícito, GORM solo refresca
// la marca de tiempo si la columna está seleccionada.
func (r *orderRepository) Update(ctx context.Context, order *model.Order) error {
	result := r.db.WithContext(ctx).
		Model(&model.Order{}).
		Where("id = ?", order.ID).
		Select(orderUpdateColumns).
		Updates(order)
	if result.Error != nil {
		return fmt.Errorf("actualizando pedido: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// Puede pasar si el pedido fue eliminado entre el FindByID y el Update.
		return model.ErrOrderNotFound
	}
	return nil
}

// Delete hace un soft delete del pedido y de sus líneas dentro de una misma
// transacción: la cabecera deja de listarse y las líneas desaparecen con ella
// (el Preload también las filtra). No se borra ninguna fila físicamente.
func (r *orderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("order_id = ?", id).Delete(&model.OrderItem{}).Error; err != nil {
			return fmt.Errorf("eliminando ítems del pedido: %w", err)
		}

		result := tx.Where("id = ?", id).Delete(&model.Order{})
		if result.Error != nil {
			return fmt.Errorf("eliminando pedido: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return model.ErrOrderNotFound
		}
		return nil
	})
}

// NextDailySequence devuelve el próximo número de secuencia libre para el
// código del pedido en el día indicado (PED-YYYYMMDD-XXXX).
//
// El sufijo se extrae con SUBSTRING a partir de la longitud del prefijo, que
// sale de model.OrderCodePrefix, y se castea a INTEGER para que la comparación
// sea numérica (9999 < 10000 aunque el orden alfabético diga lo contrario).
//
// Raw no aplica el scope del soft delete, así que también se cuentan los
// pedidos eliminados: sus códigos siguen ocupando el índice único.
func (r *orderRepository) NextDailySequence(ctx context.Context, day time.Time) (int64, error) {
	prefix := model.OrderCodePrefix(day)
	query := fmt.Sprintf(
		`SELECT COALESCE(MAX(CAST(SUBSTRING(code FROM %d) AS INTEGER)), 0) FROM orders WHERE code LIKE ?`,
		len(prefix)+1,
	)

	var last int64
	if err := r.db.WithContext(ctx).Raw(query, prefix+"%").Scan(&last).Error; err != nil {
		return 0, fmt.Errorf("calculando la secuencia diaria del pedido: %w", err)
	}
	return last + 1, nil
}

// CreateItems inserta líneas nuevas de un pedido existente. El UUID de cada
// línea lo genera model.OrderItem.BeforeCreate.
func (r *orderRepository) CreateItems(ctx context.Context, items []model.OrderItem) error {
	if len(items) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Create(&items).Error; err != nil {
		return fmt.Errorf("creando ítems del pedido: %w", err)
	}
	return nil
}

// orderItemUpdateColumns son las columnas que el PUT puede reescribir en una
// línea. Se excluyen order_id (nunca cambia) y created_at.
var orderItemUpdateColumns = []string{
	"product_id", "product_name", "unit", "quantity", "quantity_delivered",
	"unit_price", "line_total", "notes", "updated_at",
}

// UpdateItem guarda los cambios de una línea existente, acotado por order_id
// para que un id de línea ajeno no pueda modificarse.
func (r *orderRepository) UpdateItem(ctx context.Context, item *model.OrderItem) error {
	result := r.db.WithContext(ctx).
		Model(&model.OrderItem{}).
		Where("id = ? AND order_id = ?", item.ID, item.OrderID).
		Select(orderItemUpdateColumns).
		Updates(item)
	if result.Error != nil {
		return fmt.Errorf("actualizando ítem del pedido: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return model.ErrOrderItemNotFound
	}
	return nil
}

// SoftDeleteItems marca las líneas indicadas como eliminadas. No se borran
// físicamente: quedan para el histórico de entregas parciales.
func (r *orderRepository) SoftDeleteItems(ctx context.Context, orderID uuid.UUID, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).
		Where("order_id = ? AND id IN ?", orderID, ids).
		Delete(&model.OrderItem{}).Error; err != nil {
		return fmt.Errorf("eliminando ítems del pedido: %w", err)
	}
	return nil
}

// WithTransaction ejecuta fn dentro de una transacción. El repositorio que
// recibe fn comparte la transacción, así que un error deshace todos los
// cambios de la cabecera y de las líneas.
func (r *orderRepository) WithTransaction(ctx context.Context, fn func(tx repository.OrderRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(&orderRepository{db: tx})
	})
}

// translateOrderError convierte los errores de GORM en errores de dominio para
// que el service no tenga que importar gorm.
func translateOrderError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.ErrOrderNotFound
	}
	return fmt.Errorf("consultando pedido: %w", err)
}
