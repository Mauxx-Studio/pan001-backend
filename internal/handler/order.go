package handler

import (
	"errors"
	"log"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pan001/backend/internal/middleware"
	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/service"
)

// orderDateLayout es el formato de fecha sin hora que aceptan los endpoints.
const orderDateLayout = "2006-01-02"

// OrderHandler agrupa los endpoints CRUD de pedidos. Todas sus rutas se montan
// dentro del grupo protegido por el middleware JWT.
type OrderHandler struct {
	orders   *service.OrderService
	validate *validator.Validate
}

// NewOrderHandler construye el handler de pedidos.
func NewOrderHandler(orders *service.OrderService) *OrderHandler {
	return &OrderHandler{
		orders:   orders,
		validate: newValidator(),
	}
}

// Create atiende POST /api/v1/orders.
//
// El único campo obligatorio es contact_id (más las líneas cuando el pedido no
// es borrador, regla que aplica el service). El precio de cada línea se toma del
// catálogo: si el cliente lo envía, se ignora.
func (h *OrderHandler) Create(c fiber.Ctx) error {
	var req createOrderRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el body debe ser un JSON válido")
	}
	if err := h.validate.Struct(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationMessage(err))
	}

	contactID, err := parseUUIDValue("contact_id", req.ContactID)
	if err != nil {
		return err
	}
	deliveryAt, _, err := parseOrderDate("delivery_at", derefString(req.DeliveryAt))
	if err != nil {
		return err
	}

	items := make([]service.CreateOrderItemInput, 0, len(req.Items))
	for _, item := range req.Items {
		productID, err := parseUUIDValue("product_id", item.ProductID)
		if err != nil {
			return err
		}
		items = append(items, service.CreateOrderItemInput{
			ProductID: productID,
			Quantity:  item.Quantity,
			Notes:     item.Notes,
		})
	}

	order, err := h.orders.Create(c.Context(), service.CreateOrderInput{
		ContactID:      contactID,
		Status:         req.Status,
		DeliveryAt:     deliveryAt,
		DeliveryType:   req.DeliveryType,
		DiscountAmount: req.DiscountAmount,
		PaymentMethod:  req.PaymentMethod,
		PaymentStatus:  req.PaymentStatus,
		AmountPaid:     req.AmountPaid,
		Notes:          req.Notes,
		CreatedBy:      orderCreatedBy(c),
		IsActive:       req.IsActive,
		Items:          items,
	})
	if err != nil {
		return orderError("crear pedido", err)
	}

	return c.Status(fiber.StatusCreated).JSON(newOrderDetailResponse(order))
}

// List atiende GET /api/v1/orders con filtros opcionales por query string:
// contact_id, status, payment_status, from, to y search.
func (h *OrderHandler) List(c fiber.Ctx) error {
	contactID, err := queryUUID(c, "contact_id")
	if err != nil {
		return err
	}
	from, _, err := parseOrderDate("from", c.Query("from"))
	if err != nil {
		return err
	}
	to, toIsDateOnly, err := parseOrderDate("to", c.Query("to"))
	if err != nil {
		return err
	}
	// Un "to" sin hora incluye todo el día: ordered_at <= 23:59:59.999999999.
	if to != nil && toIsDateOnly {
		endOfDay := to.Add(24*time.Hour - time.Nanosecond)
		to = &endOfDay
	}

	orders, err := h.orders.List(c.Context(), service.ListOrdersInput{
		ContactID:     contactID,
		Status:        c.Query("status"),
		PaymentStatus: c.Query("payment_status"),
		From:          from,
		To:            to,
		Search:        c.Query("search"),
	})
	if err != nil {
		return orderError("listar pedidos", err)
	}

	return c.Status(fiber.StatusOK).JSON(newOrderResponses(orders))
}

// GetByID atiende GET /api/v1/orders/:id y devuelve el pedido con sus líneas.
func (h *OrderHandler) GetByID(c fiber.Ctx) error {
	id, err := orderID(c)
	if err != nil {
		return err
	}

	order, err := h.orders.GetByID(c.Context(), id)
	if err != nil {
		return orderError("obtener pedido", err)
	}

	return c.Status(fiber.StatusOK).JSON(newOrderDetailResponse(order))
}

// Update atiende PUT /api/v1/orders/:id. Solo modifica los campos enviados: los
// ausentes conservan su valor actual.
//
// items ausente deja las líneas como están; items presente (aunque sea [])
// define el conjunto final: las líneas que no aparecen se eliminan. Una línea
// con id modifica la existente y sin id agrega una nueva. delivery_at vacío
// borra la fecha programada.
func (h *OrderHandler) Update(c fiber.Ctx) error {
	id, err := orderID(c)
	if err != nil {
		return err
	}

	var req updateOrderRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el body debe ser un JSON válido")
	}
	if err := h.validate.Struct(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationMessage(err))
	}

	input := service.UpdateOrderInput{
		Status:         req.Status,
		DeliveryType:   req.DeliveryType,
		PaymentMethod:  req.PaymentMethod,
		PaymentStatus:  req.PaymentStatus,
		DiscountAmount: req.DiscountAmount,
		AmountPaid:     req.AmountPaid,
		Notes:          req.Notes,
		IsActive:       req.IsActive,
	}

	if req.ContactID != nil {
		contactID, err := parseUUIDValue("contact_id", *req.ContactID)
		if err != nil {
			return err
		}
		input.ContactID = &contactID
	}

	if req.DeliveryAt != nil {
		deliveryAt, _, err := parseOrderDate("delivery_at", *req.DeliveryAt)
		if err != nil {
			return err
		}
		if deliveryAt == nil {
			// El campo vino vacío: se limpia la fecha programada.
			deliveryAt = &time.Time{}
		}
		input.DeliveryAt = deliveryAt
	}

	if req.Items != nil {
		items := make([]service.UpdateOrderItemInput, 0, len(req.Items))
		for _, item := range req.Items {
			line := service.UpdateOrderItemInput{
				Quantity: item.Quantity,
				Notes:    item.Notes,
			}
			if item.ID != nil {
				itemID, err := parseUUIDValue("id", *item.ID)
				if err != nil {
					return err
				}
				line.ID = &itemID
			}
			if item.ProductID != nil {
				productID, err := parseUUIDValue("product_id", *item.ProductID)
				if err != nil {
					return err
				}
				line.ProductID = &productID
			}
			items = append(items, line)
		}
		input.Items = &items
	}

	order, err := h.orders.Update(c.Context(), id, input)
	if err != nil {
		return orderError("actualizar pedido", err)
	}

	return c.Status(fiber.StatusOK).JSON(newOrderDetailResponse(order))
}

// Delete atiende DELETE /api/v1/orders/:id con un soft delete de la cabecera y
// de sus líneas. Responde 204 sin body cuando el pedido existía.
func (h *OrderHandler) Delete(c fiber.Ctx) error {
	id, err := orderID(c)
	if err != nil {
		return err
	}

	if err := h.orders.Delete(c.Context(), id); err != nil {
		return orderError("eliminar pedido", err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// orderID valida el :id del path y lo convierte a UUID.
func orderID(c fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusBadRequest, "el id debe ser un UUID válido")
	}
	return id, nil
}

// parseUUIDValue convierte a UUID un string del body. El validador ya controla
// el formato, pero se valida de nuevo para poder responder con el nombre del
// campo cuando algo no cierra.
func parseUUIDValue(field, raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusBadRequest, field+" debe ser un UUID válido")
	}
	return id, nil
}

// queryUUID lee un query param UUID opcional. Devuelve nil si no se envió y 400
// si el valor no es un UUID, para no ignorar filtros mal escritos.
func queryUUID(c fiber.Ctx, name string) (*uuid.UUID, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}

	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, name+" debe ser un UUID válido")
	}
	return &id, nil
}

// derefString devuelve el valor apuntado o "" cuando el campo no se envió.
func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// parseOrderDate interpreta una fecha ISO 8601 de un body o query string.
//
// Acepta RFC3339 (2025-03-01T15:04:05Z) y fecha sola (2025-03-01, medianoche
// UTC). Devuelve nil sin error cuando el valor viene vacío. dateOnly indica que
// la fecha llegó sin hora, así los filtros "hasta" pueden incluir el día
// completo.
func parseOrderDate(field, raw string) (*time.Time, bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, false, nil
	}

	if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
		utc := parsed.UTC()
		return &utc, false, nil
	}
	if parsed, err := time.Parse(orderDateLayout, trimmed); err == nil {
		utc := parsed.UTC()
		return &utc, true, nil
	}

	return nil, false, fiber.NewError(
		fiber.StatusBadRequest,
		field+" debe ser una fecha ISO 8601 (por ejemplo 2025-03-01 o 2025-03-01T15:04:05Z)",
	)
}

// orderCreatedBy devuelve el usuario autenticado para guardarlo como creador
// del pedido. Si el contexto no trae usuario, devuelve nil (la columna es
// opcional).
func orderCreatedBy(c fiber.Ctx) *uuid.UUID {
	userID := middleware.GetUserID(c)
	if userID == uuid.Nil {
		return nil
	}
	return &userID
}

// orderError traduce los errores de dominio a errores HTTP con mensajes
// claros. Cualquier error no previsto se loguea y se devuelve tal cual para que
// el errorHandler global responda 500 sin filtrar detalles internos.
func orderError(operation string, err error) error {
	switch {
	case errors.Is(err, model.ErrOrderNotFound), errors.Is(err, model.ErrOrderItemNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, model.ErrContactNotFound):
		return fiber.NewError(fiber.StatusBadRequest, "el tercero indicado no existe")
	case errors.Is(err, model.ErrProductNotFound):
		return fiber.NewError(fiber.StatusBadRequest, "el producto indicado no existe")
	case errors.Is(err, model.ErrOrderCodeConflict):
		return fiber.NewError(fiber.StatusConflict, "no se pudo generar el código del pedido, intente de nuevo")
	case errors.Is(err, model.ErrOrderContactRequired),
		errors.Is(err, model.ErrOrderItemsRequired),
		errors.Is(err, model.ErrOrderStatusInvalid),
		errors.Is(err, model.ErrOrderDeliveryTypeInvalid),
		errors.Is(err, model.ErrOrderPaymentMethodInvalid),
		errors.Is(err, model.ErrOrderPaymentStatusInvalid),
		errors.Is(err, model.ErrOrderDiscountInvalid),
		errors.Is(err, model.ErrOrderAmountPaidInvalid),
		errors.Is(err, model.ErrOrderItemProductRequired),
		errors.Is(err, model.ErrOrderItemQuantityInvalid),
		errors.Is(err, model.ErrOrderItemDeliveredInvalid),
		errors.Is(err, model.ErrOrderItemNotInOrder),
		errors.Is(err, model.ErrOrderItemDuplicated),
		errors.Is(err, model.ErrOrderItemDeliveredLocked),
		errors.Is(err, model.ErrOrderProductPriceInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		log.Printf("error en %s: %v", operation, err)
		return err
	}
}
