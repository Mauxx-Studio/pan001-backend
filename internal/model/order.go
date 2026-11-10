package model

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Estados posibles de un pedido, en el orden en que avanza su ciclo de vida.
const (
	OrderStatusDraft              = "draft"
	OrderStatusConfirmed          = "confirmed"
	OrderStatusInProduction       = "in_production"
	OrderStatusReady              = "ready"
	OrderStatusPartiallyDelivered = "partially_delivered"
	OrderStatusDelivered          = "delivered"
	OrderStatusCancelled          = "cancelled"

	// DefaultOrderStatus es el estado con el que nace un pedido cuando el
	// cliente no envía `status`: se asume confirmado porque el alta normal
	// representa un pedido en firme. Un borrador se pide explícitamente con
	// status=draft y es el único estado que admite un pedido sin líneas.
	DefaultOrderStatus = OrderStatusConfirmed
)

// Tipos de entrega posibles. Ambos son opcionales: un pedido puede no tenerlos
// definidos mientras se coordina con el cliente.
const (
	DeliveryTypePickup   = "pickup"
	DeliveryTypeDelivery = "delivery"
)

// Medios de pago posibles. Todos son opcionales.
const (
	PaymentMethodCash           = "cash"
	PaymentMethodTransfer       = "transfer"
	PaymentMethodCard           = "card"
	PaymentMethodCurrentAccount = "current_account"
	PaymentMethodOther          = "other"
)

// Estados de pago posibles.
const (
	PaymentStatusPending = "pending"
	PaymentStatusPartial = "partial"
	PaymentStatusPaid    = "paid"

	// DefaultPaymentStatus es el estado de pago inicial: un pedido nace
	// impago y sin cobros registrados.
	DefaultPaymentStatus = PaymentStatusPending
)

// Formato del código interno del pedido: PED-YYYYMMDD-XXXX, donde XXXX es la
// secuencia diaria con ceros a la izquierda (0001, 0002, ...). El ancho mínimo
// es 4; si un día se superan los 9999 pedidos el número crece a 5 dígitos sin
// romper la unicidad.
const (
	orderCodePrefix     = "PED-"
	orderCodeDateFormat = "20060102"
)

// OrderCodePrefix devuelve el prefijo del código para el día indicado (en UTC,
// que es la zona con la que el proyecto guarda todas las fechas).
func OrderCodePrefix(day time.Time) string {
	return orderCodePrefix + day.UTC().Format(orderCodeDateFormat) + "-"
}

// NewOrderCode arma el código interno del pedido a partir del día y de la
// secuencia diaria. La unicidad la garantiza el índice único de orders.code.
func NewOrderCode(day time.Time, sequence int64) string {
	if sequence < 0 {
		sequence = 0
	}
	// %04d rellena con ceros hasta 4 dígitos y sigue creciendo si hace falta.
	return fmt.Sprintf("%s%04d", OrderCodePrefix(day), sequence)
}

// Order es la cabecera de un pedido: el tercero (idealmente cliente) que lo
// hizo, sus importes, el estado de entrega y de pago y las notas. Las líneas
// viven en OrderItem y se cargan junto con la cabecera al consultar el detalle.
//
// Al igual que el resto de entidades, es a la vez entidad de dominio y modelo
// GORM. Los importes usan shopspring/decimal con columnas NUMERIC para no pasar
// por float64 ni acumular errores de redondeo.
//
// quantity_delivered (en las líneas) y parent_order_id quedan preparados para
// las entregas parciales / división en pedidos hijos, que todavía no se
// implementan: la API no los escribe, pero el esquema ya los soporta.
type Order struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Code      string    `gorm:"type:varchar(30);not null;uniqueIndex" json:"code"`
	ContactID uuid.UUID `gorm:"column:contact_id;type:uuid;not null;index" json:"contact_id"`
	Status    string    `gorm:"type:varchar(30);not null;index" json:"status"`

	// OrderedAt se asigna al crear el pedido; DeliveryAt es la fecha/hora de
	// entrega acordada y puede quedar vacía.
	OrderedAt  time.Time  `gorm:"column:ordered_at;not null;index" json:"ordered_at"`
	DeliveryAt *time.Time `gorm:"column:delivery_at" json:"delivery_at,omitempty"`
	// DeliveryType es pickup o delivery (opcional).
	DeliveryType string `gorm:"column:delivery_type;type:varchar(20)" json:"delivery_type,omitempty"`

	// Subtotal = suma de los LineTotal de las líneas.
	// FinalAmount = Subtotal - DiscountAmount (nunca negativo).
	Subtotal       decimal.Decimal `gorm:"type:numeric(14,2);not null" json:"subtotal"`
	DiscountAmount decimal.Decimal `gorm:"column:discount_amount;type:numeric(14,2);not null" json:"discount_amount"`
	FinalAmount    decimal.Decimal `gorm:"column:final_amount;type:numeric(14,2);not null" json:"final_amount"`

	// PaymentMethod y PaymentStatus son informativos: el módulo no deriva uno
	// del otro ni calcula AmountPaid de forma automática.
	PaymentMethod string          `gorm:"column:payment_method;type:varchar(30)" json:"payment_method,omitempty"`
	PaymentStatus string          `gorm:"column:payment_status;type:varchar(20);not null" json:"payment_status"`
	AmountPaid    decimal.Decimal `gorm:"column:amount_paid;type:numeric(14,2);not null" json:"amount_paid"`

	// ParentOrderID apunta al pedido origen cuando este pedido es un pendiente
	// hijo (entregas parciales). Hoy siempre queda en NULL: la división todavía
	// no está implementada y la API no lo acepta.
	ParentOrderID *uuid.UUID `gorm:"column:parent_order_id;type:uuid;index" json:"parent_order_id,omitempty"`

	Notes string `gorm:"type:text" json:"notes,omitempty"`
	// CreatedBy es el usuario del token que creó el pedido (nullable).
	CreatedBy *uuid.UUID `gorm:"column:created_by;type:uuid" json:"created_by,omitempty"`
	IsActive  bool       `gorm:"column:is_active;not null" json:"is_active"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Items es la relación Order has many OrderItems. Se usa para crear el
	// pedido junto con sus líneas y para precargar el detalle.
	Items []OrderItem `gorm:"foreignKey:OrderID" json:"items,omitempty"`

	// Las asociaciones siguientes existen únicamente para que AutoMigrate cree
	// las claves foráneas de contact_id, parent_order_id y created_by. Nunca se
	// asignan ni se precargan, y se ocultan del JSON.
	Contact       *Contact `gorm:"foreignKey:ContactID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	ParentOrder   *Order   `gorm:"foreignKey:ParentOrderID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
	CreatedByUser *User    `gorm:"foreignKey:CreatedBy;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
}

// TableName fija el nombre de la tabla para no depender de la convención de GORM.
func (Order) TableName() string {
	return "orders"
}

// BeforeCreate genera el UUID del pedido antes de insertarlo.
//
// IsActive no lleva `default` en la etiqueta GORM a propósito (igual que en
// Contact y Product): con un default, GORM omitiría un false explícito en el
// INSERT. El valor por defecto del dominio (is_active=true) lo resuelve el
// service antes de persistir.
func (o *Order) BeforeCreate(_ *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return nil
}

// Recalculate recalcula la línea, el subtotal y el importe final del pedido.
//
// Reglas:
//   - LineTotal = Quantity * UnitPrice, redondeado a 2 decimales (centavos).
//   - Subtotal = suma de los LineTotal de las líneas cargadas en Items.
//   - FinalAmount = Subtotal - DiscountAmount.
//
// La validación de que el descuento no supere el subtotal (FinalAmount >= 0)
// queda en el service, que tiene el contexto para devolver un error claro.
func (o *Order) Recalculate() {
	subtotal := decimal.Zero
	for i := range o.Items {
		item := &o.Items[i]
		item.LineTotal = item.Quantity.Mul(item.UnitPrice).Round(2)
		subtotal = subtotal.Add(item.LineTotal)
	}
	o.Subtotal = subtotal
	o.FinalAmount = subtotal.Sub(o.DiscountAmount).Round(2)
}

// OrderItem es una línea del pedido: qué producto, cuánto y a qué precio.
//
// Se guardan snapshots de ProductName, Unit y UnitPrice para que el pedido no
// cambie si el catálogo se edita después: el precio del pedido es el vigente al
// momento de cargar la línea, no el actual del producto.
//
// QuantityDelivered arranca en 0 y se usará en las entregas parciales (todavía
// no implementadas). Las líneas usan soft delete: al reemplazar ítems en un PUT
// el registro no se borra físicamente, solo deja de listarse y se conserva para
// las entregas ya registradas.
type OrderItem struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	OrderID   uuid.UUID `gorm:"column:order_id;type:uuid;not null;index" json:"order_id"`
	ProductID uuid.UUID `gorm:"column:product_id;type:uuid;not null;index" json:"product_id"`

	// Snapshots del producto al momento de cargar la línea.
	ProductName string `gorm:"column:product_name;type:varchar(150);not null" json:"product_name"`
	Unit        string `gorm:"type:varchar(20);not null" json:"unit"`

	// numeric(14,3) para cantidades (kilos, unidades, litros) y
	// numeric(14,2) para importes, igual que en Product.
	Quantity          decimal.Decimal `gorm:"type:numeric(14,3);not null" json:"quantity"`
	QuantityDelivered decimal.Decimal `gorm:"column:quantity_delivered;type:numeric(14,3);not null" json:"quantity_delivered"`
	UnitPrice         decimal.Decimal `gorm:"column:unit_price;type:numeric(14,2);not null" json:"unit_price"`
	LineTotal         decimal.Decimal `gorm:"column:line_total;type:numeric(14,2);not null" json:"line_total"`

	Notes     string         `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Product solo existe para que AutoMigrate cree la clave foránea
	// order_items.product_id. Nunca se asigna ni se precarga.
	Product *Product `gorm:"foreignKey:ProductID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT" json:"-"`
}

// TableName fija el nombre de la tabla para no depender de la convención de GORM.
func (OrderItem) TableName() string {
	return "order_items"
}

// BeforeCreate genera el UUID de la línea antes de insertarla.
func (i *OrderItem) BeforeCreate(_ *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return nil
}

// IsValidOrderStatus indica si status es uno de los estados permitidos.
func IsValidOrderStatus(status string) bool {
	switch status {
	case OrderStatusDraft, OrderStatusConfirmed, OrderStatusInProduction,
		OrderStatusReady, OrderStatusPartiallyDelivered, OrderStatusDelivered,
		OrderStatusCancelled:
		return true
	default:
		return false
	}
}

// OrderStatuses devuelve los estados de pedido válidos.
func OrderStatuses() []string {
	return []string{
		OrderStatusDraft, OrderStatusConfirmed, OrderStatusInProduction,
		OrderStatusReady, OrderStatusPartiallyDelivered, OrderStatusDelivered,
		OrderStatusCancelled,
	}
}

// IsValidDeliveryType indica si deliveryType es pickup o delivery.
func IsValidDeliveryType(deliveryType string) bool {
	switch deliveryType {
	case DeliveryTypePickup, DeliveryTypeDelivery:
		return true
	default:
		return false
	}
}

// DeliveryTypes devuelve los tipos de entrega válidos.
func DeliveryTypes() []string {
	return []string{DeliveryTypePickup, DeliveryTypeDelivery}
}

// IsValidPaymentMethod indica si paymentMethod es un medio de pago permitido.
func IsValidPaymentMethod(paymentMethod string) bool {
	switch paymentMethod {
	case PaymentMethodCash, PaymentMethodTransfer, PaymentMethodCard,
		PaymentMethodCurrentAccount, PaymentMethodOther:
		return true
	default:
		return false
	}
}

// PaymentMethods devuelve los medios de pago válidos.
func PaymentMethods() []string {
	return []string{
		PaymentMethodCash, PaymentMethodTransfer, PaymentMethodCard,
		PaymentMethodCurrentAccount, PaymentMethodOther,
	}
}

// IsValidPaymentStatus indica si paymentStatus es pending, partial o paid.
func IsValidPaymentStatus(paymentStatus string) bool {
	switch paymentStatus {
	case PaymentStatusPending, PaymentStatusPartial, PaymentStatusPaid:
		return true
	default:
		return false
	}
}

// PaymentStatuses devuelve los estados de pago válidos.
func PaymentStatuses() []string {
	return []string{PaymentStatusPending, PaymentStatusPartial, PaymentStatusPaid}
}
