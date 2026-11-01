package handler

import (
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/service"
)

// ContactHandler agrupa los endpoints CRUD de terceros. Todas sus rutas se
// montan dentro del grupo protegido por el middleware JWT.
type ContactHandler struct {
	contacts *service.ContactService
	validate *validator.Validate
}

// NewContactHandler construye el handler de terceros.
func NewContactHandler(contacts *service.ContactService) *ContactHandler {
	return &ContactHandler{
		contacts: contacts,
		validate: newValidator(),
	}
}

// Create atiende POST /api/v1/contacts.
//
// El único campo obligatorio es name; el resto es opcional y los booleanos
// ausentes toman su valor por defecto (is_active=true).
func (h *ContactHandler) Create(c fiber.Ctx) error {
	var req createContactRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el body debe ser un JSON válido")
	}
	if err := h.validate.Struct(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationMessage(err))
	}

	contact, err := h.contacts.Create(c.Context(), service.CreateContactInput{
		Name:         req.Name,
		BusinessName: req.BusinessName,
		TaxID:        req.TaxID,
		Email:        req.Email,
		Phone:        req.Phone,
		WhatsApp:     req.WhatsApp,
		Address:      req.Address,
		City:         req.City,
		Notes:        req.Notes,
		IsClient:     req.IsClient,
		IsSupplier:   req.IsSupplier,
		IsActive:     req.IsActive,
	})
	if err != nil {
		return contactError("crear tercero", err)
	}

	return c.Status(fiber.StatusCreated).JSON(newContactResponse(contact))
}

// List atiende GET /api/v1/contacts con filtros opcionales por query string:
// is_client, is_supplier, is_active y search.
func (h *ContactHandler) List(c fiber.Ctx) error {
	isClient, err := queryBool(c, "is_client")
	if err != nil {
		return err
	}
	isSupplier, err := queryBool(c, "is_supplier")
	if err != nil {
		return err
	}
	isActive, err := queryBool(c, "is_active")
	if err != nil {
		return err
	}

	contacts, err := h.contacts.List(c.Context(), service.ListContactsInput{
		IsClient:   isClient,
		IsSupplier: isSupplier,
		IsActive:   isActive,
		Search:     c.Query("search"),
	})
	if err != nil {
		return contactError("listar terceros", err)
	}

	return c.Status(fiber.StatusOK).JSON(newContactResponses(contacts))
}

// GetByID atiende GET /api/v1/contacts/:id.
func (h *ContactHandler) GetByID(c fiber.Ctx) error {
	id, err := contactID(c)
	if err != nil {
		return err
	}

	contact, err := h.contacts.GetByID(c.Context(), id)
	if err != nil {
		return contactError("obtener tercero", err)
	}

	return c.Status(fiber.StatusOK).JSON(newContactResponse(contact))
}

// Update atiende PUT /api/v1/contacts/:id. Solo modifica los campos enviados:
// los ausentes conservan su valor actual. name, si se envía, no puede quedar
// vacío.
func (h *ContactHandler) Update(c fiber.Ctx) error {
	id, err := contactID(c)
	if err != nil {
		return err
	}

	var req updateContactRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el body debe ser un JSON válido")
	}
	if err := h.validate.Struct(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationMessage(err))
	}

	contact, err := h.contacts.Update(c.Context(), id, service.UpdateContactInput{
		Name:         req.Name,
		BusinessName: req.BusinessName,
		TaxID:        req.TaxID,
		Email:        req.Email,
		Phone:        req.Phone,
		WhatsApp:     req.WhatsApp,
		Address:      req.Address,
		City:         req.City,
		Notes:        req.Notes,
		IsClient:     req.IsClient,
		IsSupplier:   req.IsSupplier,
		IsActive:     req.IsActive,
	})
	if err != nil {
		return contactError("actualizar tercero", err)
	}

	return c.Status(fiber.StatusOK).JSON(newContactResponse(contact))
}

// Delete atiende DELETE /api/v1/contacts/:id con un soft delete. Responde 204
// sin body cuando el tercero existía.
func (h *ContactHandler) Delete(c fiber.Ctx) error {
	id, err := contactID(c)
	if err != nil {
		return err
	}

	if err := h.contacts.Delete(c.Context(), id); err != nil {
		return contactError("eliminar tercero", err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// contactID valida el :id del path y lo convierte a UUID.
func contactID(c fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusBadRequest, "el id debe ser un UUID válido")
	}
	return id, nil
}

// queryBool lee un query param booleano opcional. Devuelve nil si no se envió y
// 400 si el valor no es un booleano, para no ignorar filtros mal escritos.
func queryBool(c fiber.Ctx, name string) (*bool, error) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, nil
	}

	value, err := strconv.ParseBool(raw)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, name+" debe ser true o false")
	}
	return &value, nil
}

// contactError traduce los errores de dominio a errores HTTP con mensajes
// claros. Cualquier error no previsto se loguea y se devuelve tal cual para que
// el errorHandler global responda 500 sin filtrar detalles internos.
func contactError(operation string, err error) error {
	switch {
	case errors.Is(err, model.ErrContactNotFound):
		return fiber.NewError(fiber.StatusNotFound, "tercero no encontrado")
	case errors.Is(err, model.ErrContactNameRequired):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		log.Printf("error en %s: %v", operation, err)
		return err
	}
}
