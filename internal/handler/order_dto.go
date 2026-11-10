package handler

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"

	"github.com/pan001/backend/internal/model"
)

// createOrderItemRequest es una línea del body de POST /api/v1/orders.
//
// El precio no se recibe: se toma del catálogo al momento de cargar la línea.
// quantity_delivered tampoco se recibe (arranca en 0): las entregas parciales
// todavía no se registran desde la API.
type createOrderItemRequest struct {
	ProductID string          `json:"product_id" validate:"required,uuid"`
	Quantity  decimal.Decimal `json:"quantity"`
	Notes     string          `json:"notes" validate:"omitempty,max=2000"`
}

// createOrderRequest es el body de POST /api/v1/orders.
//
// contact_id es lo único obligatorio (además de las líneas cuando el pedido no
// es borrador, regla que aplica el service). status nace confirmed y
// payment_status nace pending. decimal.Decimal acepta tanto números (1500.5)
// como cadenas ("1500.50") en el JSON.
type createOrderRequest struct {
	ContactID      string                   `json:"contact_id" validate:"required,uuid"`
	Status         string                   `json:"status" validate:"omitempty,order_status"`
	DeliveryAt     *string                  `json:"delivery_at"`
	DeliveryType   string                   `json:"delivery_type" validate:"omitempty,delivery_type"`
	DiscountAmount *decimal.Decimal         `json:"discount_amount"`
	PaymentMethod  string                   `json:"payment_method" validate:"omitempty,payment_method"`
	PaymentStatus  string                   `json:"payment_status" validate:"omitempty,payment_status"`
	AmountPaid     *decimal.Decimal         `json:"amount_paid"`
	Notes          string                   `json:"notes" validate:"omitempty,max=2000"`
	IsActive       *bool                    `json:"is_active"`
	Items          []createOrderItemRequest `json:"items" validate:"dive"`
}

// updateOrderItemRequest es una línea del body de PUT /api/v1/orders/:id.
//
// Con id modifica una línea existente (los demás campos son opcionales); sin id
// agrega una línea nueva, y entonces product_id y quantity son obligatorios
// (lo valida el service para poder responder con un mensaje unificado).
type updateOrderItemRequest struct {
	ID        *string          `json:"id" validate:"omitempty,uuid"`
	ProductID *string          `json:"product_id" validate:"omitempty,uuid"`
	Quantity  *decimal.Decimal `json:"quantity"`
	Notes     *string          `json:"notes" validate:"omitempty,max=2000"`
}

// updateOrderRequest es el body de PUT /api/v1/orders/:id.
//
// Todos los campos son opcionales: solo se actualizan los punteros no nulos. Si
// items no viene, las líneas quedan como están; si viene (aunque sea []), define
// el conjunto final de líneas del pedido.
type updateOrderRequest struct {
	ContactID      *string                  `json:"contact_id" validate:"omitempty,uuid"`
	Status         *string                  `json:"status" validate:"omitempty,order_status"`
	DeliveryAt     *string                  `json:"delivery_at"`
	DeliveryType   *string                  `json:"delivery_type" validate:"omitempty,delivery_type"`
	DiscountAmount *decimal.Decimal         `json:"discount_amount"`
	PaymentMethod  *string                  `json:"payment_method" validate:"omitempty,payment_method"`
	PaymentStatus  *string                  `json:"payment_status" validate:"omitempty,payment_status"`
	AmountPaid     *decimal.Decimal         `json:"amount_paid"`
	Notes          *string                  `json:"notes" validate:"omitempty,max=2000"`
	IsActive       *bool                    `json:"is_active"`
	Items          []updateOrderItemRequest `json:"items" validate:"dive"`
}

// orderItemResponse es una línea del pedido en las respuestas.
type orderItemResponse struct {
	ID                string      `json:"id"`
	ProductID         string      `json:"product_id"`
	ProductName       string      `json:"product_name"`
	Unit              string      `json:"unit"`
	Quantity          json.Number `json:"quantity"`
	QuantityDelivered json.Number `json:"quantity_delivered"`
	UnitPrice         json.Number `json:"unit_price"`
	LineTotal         json.Number `json:"line_total"`
	Notes             string      `json:"notes,omitempty"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

// orderResponse es el resumen público de un pedido. Es lo que devuelve el
// listado: la cabecera sin sus líneas.
//
// Los importes se devuelven como números JSON (json.Number) con los dígitos
// exactos que guarda la base, igual que en productos: no se convierten a
// float64 para no perder precisión.
type orderResponse struct {
	ID             string      `json:"id"`
	Code           string      `json:"code"`
	ContactID      string      `json:"contact_id"`
	Status         string      `json:"status"`
	OrderedAt      time.Time   `json:"ordered_at"`
	DeliveryAt     *time.Time  `json:"delivery_at,omitempty"`
	DeliveryType   string      `json:"delivery_type,omitempty"`
	Subtotal       json.Number `json:"subtotal"`
	DiscountAmount json.Number `json:"discount_amount"`
	FinalAmount    json.Number `json:"final_amount"`
	PaymentMethod  string      `json:"payment_method,omitempty"`
	PaymentStatus  string      `json:"payment_status"`
	AmountPaid     json.Number `json:"amount_paid"`
	// ParentOrderID queda vacío hasta que existan las entregas parciales.
	ParentOrderID string    `json:"parent_order_id,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	CreatedBy     string    `json:"created_by,omitempty"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// orderDetailResponse es el detalle de un pedido: el resumen más sus líneas.
// Los campos del resumen se embeben, así el JSON queda plano.
type orderDetailResponse struct {
	orderResponse
	Items []orderItemResponse `json:"items"`
}

// newOrderItemResponse mapea una línea de dominio al DTO de salida.
func newOrderItemResponse(item *model.OrderItem) orderItemResponse {
	return orderItemResponse{
		ID:                item.ID.String(),
		ProductID:         item.ProductID.String(),
		ProductName:       item.ProductName,
		Unit:              item.Unit,
		Quantity:          json.Number(item.Quantity.String()),
		QuantityDelivered: json.Number(item.QuantityDelivered.String()),
		UnitPrice:         json.Number(item.UnitPrice.String()),
		LineTotal:         json.Number(item.LineTotal.String()),
		Notes:             item.Notes,
		CreatedAt:         item.CreatedAt.UTC(),
		UpdatedAt:         item.UpdatedAt.UTC(),
	}
}

// newOrderResponse mapea la cabecera de dominio al DTO de salida.
func newOrderResponse(order *model.Order) orderResponse {
	response := orderResponse{
		ID:             order.ID.String(),
		Code:           order.Code,
		ContactID:      order.ContactID.String(),
		Status:         order.Status,
		OrderedAt:      order.OrderedAt.UTC(),
		DeliveryAt:     utcTimePointer(order.DeliveryAt),
		DeliveryType:   order.DeliveryType,
		Subtotal:       json.Number(order.Subtotal.String()),
		DiscountAmount: json.Number(order.DiscountAmount.String()),
		FinalAmount:    json.Number(order.FinalAmount.String()),
		PaymentMethod:  order.PaymentMethod,
		PaymentStatus:  order.PaymentStatus,
		AmountPaid:     json.Number(order.AmountPaid.String()),
		Notes:          order.Notes,
		IsActive:       order.IsActive,
		CreatedAt:      order.CreatedAt.UTC(),
		UpdatedAt:      order.UpdatedAt.UTC(),
	}
	if order.ParentOrderID != nil {
		response.ParentOrderID = order.ParentOrderID.String()
	}
	if order.CreatedBy != nil {
		response.CreatedBy = order.CreatedBy.String()
	}
	return response
}

// newOrderResponses mapea una lista de pedidos al DTO de salida. Devuelve un
// slice vacío (no nil) para que la API responda [] y no null.
func newOrderResponses(orders []model.Order) []orderResponse {
	responses := make([]orderResponse, 0, len(orders))
	for i := range orders {
		responses = append(responses, newOrderResponse(&orders[i]))
	}
	return responses
}

// newOrderDetailResponse mapea un pedido con sus líneas.
func newOrderDetailResponse(order *model.Order) orderDetailResponse {
	items := make([]orderItemResponse, 0, len(order.Items))
	for i := range order.Items {
		items = append(items, newOrderItemResponse(&order.Items[i]))
	}
	return orderDetailResponse{
		orderResponse: newOrderResponse(order),
		Items:         items,
	}
}

// utcTimePointer normaliza a UTC una fecha opcional.
func utcTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}
