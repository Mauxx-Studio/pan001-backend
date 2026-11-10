package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

const (
	// DefaultOrderIsActive es true: los pedidos nacen activos.
	DefaultOrderIsActive = true
	// orderCodeAttempts es cuántas veces se reintenta crear el pedido cuando
	// otro request se quedó con el mismo número de secuencia diaria.
	orderCodeAttempts = 5
	// quantityScale son los decimales de las cantidades (numeric(14,3)).
	quantityScale = 3
	// amountScale son los decimales de los importes (numeric(14,2) = centavos).
	amountScale = 2
)

// CreateOrderItemInput es una línea a agregar al crear el pedido.
//
// El precio no se recibe: se toma del catálogo al momento de cargar la línea.
type CreateOrderItemInput struct {
	ProductID uuid.UUID
	Quantity  decimal.Decimal
	Notes     string
}

// CreateOrderInput son los datos para dar de alta un pedido.
//
// Status, DeliveryType, PaymentMethod y PaymentStatus son opcionales; el
// service resuelve los valores por defecto y valida los envíados. Los decimales
// son punteros para distinguir "no enviado" (nil) de 0 explícito.
type CreateOrderInput struct {
	ContactID      uuid.UUID
	Status         string
	DeliveryAt     *time.Time
	DeliveryType   string
	DiscountAmount *decimal.Decimal
	PaymentMethod  string
	PaymentStatus  string
	AmountPaid     *decimal.Decimal
	Notes          string
	CreatedBy      *uuid.UUID
	IsActive       *bool
	Items          []CreateOrderItemInput
}

// UpdateOrderItemInput es una línea dentro de un PUT.
//
// Sale de la línea del body: con ID se modifica una existente (quantity,
// quantity_delivered, product_id y notes son opcionales) y sin ID se agrega una
// nueva (entonces product_id y quantity son obligatorios).
type UpdateOrderItemInput struct {
	ID                *uuid.UUID
	ProductID         *uuid.UUID
	Quantity          *decimal.Decimal
	QuantityDelivered *decimal.Decimal
	Notes             *string
}

// UpdateOrderInput son los cambios de la cabecera. Un puntero en nil significa
// "no tocar".
//
// Items en nil deja las líneas como están; un slice (aunque venga vacío)
// reemplaza el conjunto completo: las líneas que no aparecen se eliminan.
type UpdateOrderInput struct {
	ContactID      *uuid.UUID
	Status         *string
	DeliveryAt     *time.Time
	DeliveryType   *string
	DiscountAmount *decimal.Decimal
	PaymentMethod  *string
	PaymentStatus  *string
	AmountPaid     *decimal.Decimal
	Notes          *string
	IsActive       *bool
	Items          *[]UpdateOrderItemInput
}

// ListOrdersInput son los filtros opcionales del listado. From y To acotan
// ordered_at (ambos inclusive) y Search busca en code, notes y nombre del
// tercero.
type ListOrdersInput struct {
	ContactID     *uuid.UUID
	Status        string
	PaymentStatus string
	From          *time.Time
	To            *time.Time
	Search        string
}

// OrderService agrupa los casos de uso de pedidos.
//
// Depende del catálogo y de los terceros solo para validar referencias y tomar
// snapshots: no modifica ni productos ni contactos.
type OrderService struct {
	orders   repository.OrderRepository
	contacts repository.ContactRepository
	products repository.ProductRepository
}

// NewOrderService construye el servicio de pedidos.
func NewOrderService(
	orders repository.OrderRepository,
	contacts repository.ContactRepository,
	products repository.ProductRepository,
) *OrderService {
	return &OrderService{orders: orders, contacts: contacts, products: products}
}

// Create da de alta un pedido con sus líneas en una sola transacción.
//
// Reglas:
//   - El tercero es obligatorio y tiene que existir (no estar eliminado).
//   - status nace en confirmed salvo que se pida draft. Un pedido que no es
//     borrador necesita al menos una línea; el borrador puede quedar vacío.
//   - delivery_type, payment_method y payment_status son opcionales y se
//     validan contra las constantes del dominio; payment_status nace pending.
//   - Cada línea exige un producto existente, con precio de venta mayor que
//     cero, y una cantidad mayor que cero.
//   - El precio se copia del catálogo (snapshot) y nunca se recibe del cliente.
//   - discount_amount no puede ser negativo ni superar el subtotal.
//   - El código PED-YYYYMMDD-XXXX se genera al final, dentro de la transacción,
//     y se reintenta si otra petición simultánea se quedó con el mismo número.
func (s *OrderService) Create(ctx context.Context, in CreateOrderInput) (*model.Order, error) {
	if in.ContactID == uuid.Nil {
		return nil, model.ErrOrderContactRequired
	}

	status, err := optionalEnumValue(in.Status, model.DefaultOrderStatus, model.IsValidOrderStatus, model.ErrOrderStatusInvalid)
	if err != nil {
		return nil, err
	}
	deliveryType, err := optionalEnumValue(in.DeliveryType, "", model.IsValidDeliveryType, model.ErrOrderDeliveryTypeInvalid)
	if err != nil {
		return nil, err
	}
	paymentMethod, err := optionalEnumValue(in.PaymentMethod, "", model.IsValidPaymentMethod, model.ErrOrderPaymentMethodInvalid)
	if err != nil {
		return nil, err
	}
	paymentStatus, err := optionalEnumValue(in.PaymentStatus, model.DefaultPaymentStatus, model.IsValidPaymentStatus, model.ErrOrderPaymentStatusInvalid)
	if err != nil {
		return nil, err
	}

	discount := decimalOr(in.DiscountAmount).Round(amountScale)
	if discount.IsNegative() {
		return nil, model.ErrOrderDiscountInvalid
	}
	amountPaid := decimalOr(in.AmountPaid).Round(amountScale)
	if amountPaid.IsNegative() {
		return nil, model.ErrOrderAmountPaidInvalid
	}

	// Verificamos el tercero para responder con un 400 claro; la clave foránea
	// de la tabla igual protege la integridad a nivel base.
	if _, err := s.contacts.FindByID(ctx, in.ContactID); err != nil {
		return nil, err
	}

	if status != model.OrderStatusDraft && len(in.Items) == 0 {
		return nil, model.ErrOrderItemsRequired
	}

	orderedAt := time.Now().UTC()
	order := &model.Order{
		ContactID:      in.ContactID,
		Status:         status,
		OrderedAt:      orderedAt,
		DeliveryAt:     utcTimeOrNil(in.DeliveryAt),
		DeliveryType:   deliveryType,
		DiscountAmount: discount,
		PaymentMethod:  paymentMethod,
		PaymentStatus:  paymentStatus,
		AmountPaid:     amountPaid,
		Notes:          strings.TrimSpace(in.Notes),
		CreatedBy:      in.CreatedBy,
		IsActive:       boolOr(in.IsActive, DefaultOrderIsActive),
	}

	items := make([]model.OrderItem, 0, len(in.Items))
	for _, itemIn := range in.Items {
		item, err := s.newOrderItem(ctx, itemIn.ProductID, itemIn.Quantity, itemIn.Notes)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	order.Items = items
	order.Recalculate()
	if order.FinalAmount.IsNegative() {
		return nil, model.ErrOrderDiscountInvalid
	}

	if err := s.persist(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// persist guarda el pedido nuevo generando el código dentro de la transacción.
// Si la secuencia diaria choca con el índice único (dos peticiones a la vez en
// el mismo día) se reintenta leyendo la secuencia de nuevo.
func (s *OrderService) persist(ctx context.Context, order *model.Order) error {
	var err error
	for attempt := 0; attempt < orderCodeAttempts; attempt++ {
		err = s.orders.WithTransaction(ctx, func(tx repository.OrderRepository) error {
			sequence, seqErr := tx.NextDailySequence(ctx, order.OrderedAt)
			if seqErr != nil {
				return seqErr
			}
			order.Code = model.NewOrderCode(order.OrderedAt, sequence)
			return tx.Create(ctx, order)
		})
		if err == nil {
			return nil
		}
		// Solo se reintenta si otro pedido se quedó con el mismo código.
		if !errors.Is(err, model.ErrOrderCodeConflict) {
			return err
		}
	}
	return err
}

// GetByID devuelve un pedido con sus líneas.
//
// Reglas:
//   - Si no existe o fue soft-deleted → model.ErrOrderNotFound.
func (s *OrderService) GetByID(ctx context.Context, id uuid.UUID) (*model.Order, error) {
	if id == uuid.Nil {
		return nil, model.ErrOrderNotFound
	}

	return s.orders.FindByID(ctx, id)
}

// List devuelve los pedidos no eliminados con los filtros indicados. El
// resultado nunca es nil, así la API responde [] y no null.
//
// Reglas:
//   - status y payment_status, si vienen, tienen que ser valores del dominio.
//   - from y to acotan ordered_at (ambos inclusive).
func (s *OrderService) List(ctx context.Context, in ListOrdersInput) ([]model.Order, error) {
	status := strings.ToLower(strings.TrimSpace(in.Status))
	if status != "" && !model.IsValidOrderStatus(status) {
		return nil, model.ErrOrderStatusInvalid
	}
	paymentStatus := strings.ToLower(strings.TrimSpace(in.PaymentStatus))
	if paymentStatus != "" && !model.IsValidPaymentStatus(paymentStatus) {
		return nil, model.ErrOrderPaymentStatusInvalid
	}

	orders, err := s.orders.List(ctx, repository.OrderFilters{
		ContactID:     in.ContactID,
		Status:        status,
		PaymentStatus: paymentStatus,
		From:          utcTimeOrNil(in.From),
		To:            utcTimeOrNil(in.To),
		Search:        strings.TrimSpace(in.Search),
	})
	if err != nil {
		return nil, err
	}
	if orders == nil {
		return []model.Order{}, nil
	}
	return orders, nil
}

// Update modifica el pedido y, si se envían items, reemplaza el conjunto de
// líneas. Todo se confirma en una sola transacción.
//
// Reglas:
//   - El pedido tiene que existir → model.ErrOrderNotFound.
//   - Si cambia contact_id, el tercero tiene que existir.
//   - Los enums se validan contra las constantes del dominio; delivery_type y
//     payment_method se pueden vaciar con "" y status/payment_status no.
//   - items ausente deja las líneas como están; items presente (aunque sea [])
//     define el conjunto final. Las líneas que no aparecen se quitan y las que
//     ya tienen cantidad entregada no se pueden quitar.
//   - Una línea existente se identifica por id; sin id se agrega una nueva.
//   - Una cantidad no puede quedar por debajo de lo ya entregado.
//   - Los totales se recalculan siempre y el descuento no puede superar el
//     subtotal.
//   - Un pedido que no es borrador no puede quedar sin líneas.
func (s *OrderService) Update(ctx context.Context, id uuid.UUID, in UpdateOrderInput) (*model.Order, error) {
	if id == uuid.Nil {
		return nil, model.ErrOrderNotFound
	}

	order, err := s.orders.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.ContactID != nil {
		if *in.ContactID == uuid.Nil {
			return nil, model.ErrOrderContactRequired
		}
		if _, err := s.contacts.FindByID(ctx, *in.ContactID); err != nil {
			return nil, err
		}
		order.ContactID = *in.ContactID
	}
	if in.Status != nil {
		status, err := normalizeEnumValue(*in.Status, model.IsValidOrderStatus, model.ErrOrderStatusInvalid)
		if err != nil {
			return nil, err
		}
		order.Status = status
	}
	if in.DeliveryAt != nil {
		order.DeliveryAt = utcTimeOrNil(in.DeliveryAt)
	}
	if in.DeliveryType != nil {
		deliveryType, err := clearableEnumValue(*in.DeliveryType, model.IsValidDeliveryType, model.ErrOrderDeliveryTypeInvalid)
		if err != nil {
			return nil, err
		}
		order.DeliveryType = deliveryType
	}
	if in.PaymentMethod != nil {
		paymentMethod, err := clearableEnumValue(*in.PaymentMethod, model.IsValidPaymentMethod, model.ErrOrderPaymentMethodInvalid)
		if err != nil {
			return nil, err
		}
		order.PaymentMethod = paymentMethod
	}
	if in.PaymentStatus != nil {
		paymentStatus, err := normalizeEnumValue(*in.PaymentStatus, model.IsValidPaymentStatus, model.ErrOrderPaymentStatusInvalid)
		if err != nil {
			return nil, err
		}
		order.PaymentStatus = paymentStatus
	}
	if in.DiscountAmount != nil {
		discount := in.DiscountAmount.Round(amountScale)
		if discount.IsNegative() {
			return nil, model.ErrOrderDiscountInvalid
		}
		order.DiscountAmount = discount
	}
	if in.AmountPaid != nil {
		amountPaid := in.AmountPaid.Round(amountScale)
		if amountPaid.IsNegative() {
			return nil, model.ErrOrderAmountPaidInvalid
		}
		order.AmountPaid = amountPaid
	}
	if in.Notes != nil {
		order.Notes = strings.TrimSpace(*in.Notes)
	}
	if in.IsActive != nil {
		order.IsActive = *in.IsActive
	}

	plan := &orderItemsPlan{}
	if in.Items != nil {
		plan, err = s.planOrderItems(ctx, order, *in.Items)
		if err != nil {
			return nil, err
		}
		order.Items = plan.active
	}

	order.Recalculate()
	if order.FinalAmount.IsNegative() {
		return nil, model.ErrOrderDiscountInvalid
	}
	if order.Status != model.OrderStatusDraft && len(order.Items) == 0 {
		return nil, model.ErrOrderItemsRequired
	}

	err = s.orders.WithTransaction(ctx, func(tx repository.OrderRepository) error {
		if err := tx.Update(ctx, order); err != nil {
			return err
		}
		for i := range plan.updated {
			if err := tx.UpdateItem(ctx, &plan.updated[i]); err != nil {
				return err
			}
		}
		if err := tx.CreateItems(ctx, plan.added); err != nil {
			return err
		}
		return tx.SoftDeleteItems(ctx, order.ID, plan.removed)
	})
	if err != nil {
		return nil, err
	}

	// Se relee para devolver las líneas nuevas ya con su id y los totales
	// guardados, no los de memoria.
	return s.orders.FindByID(ctx, id)
}

// Delete hace un soft delete del pedido y de sus líneas.
//
// Reglas:
//   - Si no existe o ya estaba eliminado → model.ErrOrderNotFound.
func (s *OrderService) Delete(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return model.ErrOrderNotFound
	}

	return s.orders.Delete(ctx, id)
}

// orderItemsPlan describe los cambios que hay que aplicar sobre las líneas.
type orderItemsPlan struct {
	// active son las líneas que quedan en el pedido (existentes y nuevas). Es
	// el conjunto con el que se recalculan los totales.
	active []model.OrderItem
	// updated son líneas existentes modificadas.
	updated []model.OrderItem
	// added son líneas nuevas, todavía sin id.
	added []model.OrderItem
	// removed son los ids de las líneas que se quitan (soft delete).
	removed []uuid.UUID
}

// planOrderItems compara las líneas recibidas con las actuales y arma el plan.
//
// Una línea con id modifica la existente (el id tiene que pertenecer al pedido);
// una línea sin id agrega un producto nuevo. Las líneas actuales que no vienen
// en el body se quitan, siempre que no tengan cantidad entregada.
func (s *OrderService) planOrderItems(ctx context.Context, order *model.Order, in []UpdateOrderItemInput) (*orderItemsPlan, error) {
	existing := make(map[uuid.UUID]*model.OrderItem, len(order.Items))
	for i := range order.Items {
		existing[order.Items[i].ID] = &order.Items[i]
	}

	plan := &orderItemsPlan{
		active:  make([]model.OrderItem, 0, len(in)),
		updated: make([]model.OrderItem, 0, len(in)),
		added:   make([]model.OrderItem, 0, len(in)),
		removed: make([]uuid.UUID, 0),
	}
	seen := make(map[uuid.UUID]bool, len(in))

	for _, itemIn := range in {
		if itemIn.ID != nil {
			current, ok := existing[*itemIn.ID]
			if !ok {
				return nil, model.ErrOrderItemNotInOrder
			}
			if seen[current.ID] {
				return nil, model.ErrOrderItemDuplicated
			}
			seen[current.ID] = true

			// Se copia para no mutar el pedido leído hasta validar todo.
			updated := *current
			if itemIn.ProductID != nil && *itemIn.ProductID != updated.ProductID {
				product, price, err := s.loadProduct(ctx, *itemIn.ProductID)
				if err != nil {
					return nil, err
				}
				updated.ProductID = product.ID
				updated.ProductName = product.Name
				updated.Unit = product.Unit
				updated.UnitPrice = price
			}
			if itemIn.Quantity != nil {
				updated.Quantity = itemIn.Quantity.Round(quantityScale)
			}
			if itemIn.Notes != nil {
				updated.Notes = strings.TrimSpace(*itemIn.Notes)
			}
			if err := validateOrderItemQuantities(updated.Quantity, updated.QuantityDelivered); err != nil {
				return nil, err
			}
			updated.LineTotal = updated.Quantity.Mul(updated.UnitPrice).Round(amountScale)

			plan.updated = append(plan.updated, updated)
			plan.active = append(plan.active, updated)
			continue
		}

		if itemIn.ProductID == nil {
			return nil, model.ErrOrderItemProductRequired
		}
		quantity := decimal.Zero
		if itemIn.Quantity != nil {
			quantity = *itemIn.Quantity
		}
		notes := ""
		if itemIn.Notes != nil {
			notes = *itemIn.Notes
		}

		item, err := s.newOrderItem(ctx, *itemIn.ProductID, quantity, notes)
		if err != nil {
			return nil, err
		}
		item.OrderID = order.ID

		plan.added = append(plan.added, *item)
		plan.active = append(plan.active, *item)
	}

	for i := range order.Items {
		line := &order.Items[i]
		if seen[line.ID] {
			continue
		}
		// Una línea con entregas no se puede sacar: primero hay que revertir la
		// entrega, así no se pierde el historial.
		if line.QuantityDelivered.IsPositive() {
			return nil, fmt.Errorf("%w: %s", model.ErrOrderItemDeliveredLocked, line.ProductName)
		}
		plan.removed = append(plan.removed, line.ID)
	}

	return plan, nil
}

// newOrderItem arma una línea nueva tomando del catálogo el nombre, la unidad y
// el precio de venta (snapshot). La cantidad entregada arranca en cero: las
// entregas parciales todavía no se registran.
func (s *OrderService) newOrderItem(ctx context.Context, productID uuid.UUID, quantity decimal.Decimal, notes string) (*model.OrderItem, error) {
	if productID == uuid.Nil {
		return nil, model.ErrOrderItemProductRequired
	}
	qty := quantity.Round(quantityScale)
	if !qty.IsPositive() {
		return nil, model.ErrOrderItemQuantityInvalid
	}

	product, price, err := s.loadProduct(ctx, productID)
	if err != nil {
		return nil, err
	}

	return &model.OrderItem{
		ProductID:         product.ID,
		ProductName:       product.Name,
		Unit:              product.Unit,
		Quantity:          qty,
		QuantityDelivered: decimal.Zero,
		UnitPrice:         price,
		LineTotal:         qty.Mul(price).Round(amountScale),
		Notes:             strings.TrimSpace(notes),
	}, nil
}

// loadProduct busca el producto y devuelve su precio de venta redondeado.
//
// Un producto sin precio (sale_price en cero) no se puede cotizar: la línea
// quedaría en cero y el pedido no tendría sentido. Un producto inactivo sí se
// acepta, para no bloquear pedidos que ya lo tenían cargado.
func (s *OrderService) loadProduct(ctx context.Context, id uuid.UUID) (*model.Product, decimal.Decimal, error) {
	product, err := s.products.FindByID(ctx, id)
	if err != nil {
		return nil, decimal.Zero, err
	}

	price := product.SalePrice.Round(amountScale)
	if !price.IsPositive() {
		return nil, decimal.Zero, fmt.Errorf("%w: %s", model.ErrOrderProductPriceInvalid, product.Name)
	}
	return product, price, nil
}

// validateOrderItemQuantities valida la relación entre lo pedido y lo entregado.
//
// Reglas:
//   - quantity > 0.
//   - 0 <= quantity_delivered <= quantity: una línea no puede tener entregado
//     más de lo pedido y bajar la cantidad por debajo de lo entregado dejaría
//     la línea en un estado imposible.
func validateOrderItemQuantities(quantity, delivered decimal.Decimal) error {
	if !quantity.IsPositive() {
		return model.ErrOrderItemQuantityInvalid
	}
	if delivered.IsNegative() || delivered.GreaterThan(quantity) {
		return model.ErrOrderItemDeliveredInvalid
	}
	return nil
}

// normalizeEnumValue recorta, pasa a minúsculas y valida un enum obligatorio.
func normalizeEnumValue(value string, isValid func(string) bool, invalidErr error) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if !isValid(normalized) {
		return "", invalidErr
	}
	return normalized, nil
}

// optionalEnumValue valida un enum opcional y devuelve fallback si viene vacío.
func optionalEnumValue(value, fallback string, isValid func(string) bool, invalidErr error) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return fallback, nil
	}
	if !isValid(normalized) {
		return "", invalidErr
	}
	return normalized, nil
}

// clearableEnumValue valida un enum opcional que además se puede vaciar (por
// ejemplo delivery_type, para volver a "sin definir").
func clearableEnumValue(value string, isValid func(string) bool, invalidErr error) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", nil
	}
	if !isValid(normalized) {
		return "", invalidErr
	}
	return normalized, nil
}

// utcTimeOrNil normaliza una fecha opcional a UTC. Nil y la fecha cero se
// devuelven como nil para no guardar 0001-01-01 en la base.
func utcTimeOrNil(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	utc := value.UTC()
	return &utc
}
