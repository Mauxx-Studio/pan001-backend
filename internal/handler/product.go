package handler

import (
	"errors"
	"log"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/service"
)

// ProductHandler agrupa los endpoints CRUD del catálogo de productos. Todas sus
// rutas se montan dentro del grupo protegido por el middleware JWT.
type ProductHandler struct {
	products *service.ProductService
	validate *validator.Validate
}

// NewProductHandler construye el handler de productos.
func NewProductHandler(products *service.ProductService) *ProductHandler {
	return &ProductHandler{
		products: products,
		validate: newValidator(),
	}
}

// Create atiende POST /api/v1/products.
//
// Solo name y unit son obligatorios; el resto es opcional y los valores por
// defecto (track_stock=true, is_active=true) los resuelve el service.
func (h *ProductHandler) Create(c fiber.Ctx) error {
	var req createProductRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el body debe ser un JSON válido")
	}
	if err := h.validate.Struct(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationMessage(err))
	}

	product, err := h.products.Create(c.Context(), service.CreateProductInput{
		Name:         req.Name,
		SKU:          req.SKU,
		Description:  req.Description,
		Unit:         req.Unit,
		IsSold:       req.IsSold,
		IsPurchased:  req.IsPurchased,
		IsProduced:   req.IsProduced,
		TrackStock:   req.TrackStock,
		CurrentStock: req.CurrentStock,
		MinStock:     req.MinStock,
		SalePrice:    req.SalePrice,
		Cost:         req.Cost,
		IsActive:     req.IsActive,
	})
	if err != nil {
		return productError("crear producto", err)
	}

	return c.Status(fiber.StatusCreated).JSON(newProductResponse(product))
}

// List atiende GET /api/v1/products con filtros opcionales por query string:
// is_sold, is_purchased, is_produced, is_active, track_stock y search.
func (h *ProductHandler) List(c fiber.Ctx) error {
	isSold, err := queryBool(c, "is_sold")
	if err != nil {
		return err
	}
	isPurchased, err := queryBool(c, "is_purchased")
	if err != nil {
		return err
	}
	isProduced, err := queryBool(c, "is_produced")
	if err != nil {
		return err
	}
	isActive, err := queryBool(c, "is_active")
	if err != nil {
		return err
	}
	trackStock, err := queryBool(c, "track_stock")
	if err != nil {
		return err
	}

	products, err := h.products.List(c.Context(), service.ListProductsInput{
		IsSold:      isSold,
		IsPurchased: isPurchased,
		IsProduced:  isProduced,
		IsActive:    isActive,
		TrackStock:  trackStock,
		Search:      c.Query("search"),
	})
	if err != nil {
		return productError("listar productos", err)
	}

	return c.Status(fiber.StatusOK).JSON(newProductResponses(products))
}

// GetByID atiende GET /api/v1/products/:id.
func (h *ProductHandler) GetByID(c fiber.Ctx) error {
	id, err := productID(c)
	if err != nil {
		return err
	}

	product, err := h.products.GetByID(c.Context(), id)
	if err != nil {
		return productError("obtener producto", err)
	}

	return c.Status(fiber.StatusOK).JSON(newProductResponse(product))
}

// Update atiende PUT /api/v1/products/:id. Solo modifica los campos enviados:
// los ausentes conservan su valor actual. name y unit, si se envían, no pueden
// quedar vacíos (lo valida el service).
func (h *ProductHandler) Update(c fiber.Ctx) error {
	id, err := productID(c)
	if err != nil {
		return err
	}

	var req updateProductRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el body debe ser un JSON válido")
	}
	if err := h.validate.Struct(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationMessage(err))
	}

	product, err := h.products.Update(c.Context(), id, service.UpdateProductInput{
		Name:         req.Name,
		SKU:          req.SKU,
		Description:  req.Description,
		Unit:         req.Unit,
		IsSold:       req.IsSold,
		IsPurchased:  req.IsPurchased,
		IsProduced:   req.IsProduced,
		TrackStock:   req.TrackStock,
		CurrentStock: req.CurrentStock,
		MinStock:     req.MinStock,
		SalePrice:    req.SalePrice,
		Cost:         req.Cost,
		IsActive:     req.IsActive,
	})
	if err != nil {
		return productError("actualizar producto", err)
	}

	return c.Status(fiber.StatusOK).JSON(newProductResponse(product))
}

// Delete atiende DELETE /api/v1/products/:id con un soft delete. Responde 204
// sin body cuando el producto existía.
func (h *ProductHandler) Delete(c fiber.Ctx) error {
	id, err := productID(c)
	if err != nil {
		return err
	}

	if err := h.products.Delete(c.Context(), id); err != nil {
		return productError("eliminar producto", err)
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// productID valida el :id del path y lo convierte a UUID.
func productID(c fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusBadRequest, "el id debe ser un UUID válido")
	}
	return id, nil
}

// productError traduce los errores de dominio a errores HTTP con mensajes
// claros. Cualquier error no previsto se loguea y se devuelve tal cual para que
// el errorHandler global responda 500 sin filtrar detalles internos.
func productError(operation string, err error) error {
	switch {
	case errors.Is(err, model.ErrProductNotFound):
		return fiber.NewError(fiber.StatusNotFound, "producto no encontrado")
	case errors.Is(err, model.ErrProductNameRequired), errors.Is(err, model.ErrProductUnitRequired):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		log.Printf("error en %s: %v", operation, err)
		return err
	}
}
