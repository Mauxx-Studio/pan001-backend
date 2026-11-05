package handler

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"

	"github.com/pan001/backend/internal/model"
)

// createProductRequest es el body de POST /api/v1/products.
//
// Name y Unit son los únicos campos obligatorios. Los booleanos y los decimales
// son punteros para distinguir "no enviado" de false/0: el service resuelve
// track_stock=true, is_active=true, el resto false y los importes en cero.
// decimal.Decimal acepta tanto números (500) como cadenas ("500.00") en el JSON.
type createProductRequest struct {
	Name         string           `json:"name" validate:"required,max=150"`
	SKU          string           `json:"sku" validate:"omitempty,max=50"`
	Description  string           `json:"description" validate:"omitempty,max=2000"`
	Unit         string           `json:"unit" validate:"required,max=20"`
	IsSold       *bool            `json:"is_sold"`
	IsPurchased  *bool            `json:"is_purchased"`
	IsProduced   *bool            `json:"is_produced"`
	TrackStock   *bool            `json:"track_stock"`
	CurrentStock *decimal.Decimal `json:"current_stock"`
	MinStock     *decimal.Decimal `json:"min_stock"`
	SalePrice    *decimal.Decimal `json:"sale_price"`
	Cost         *decimal.Decimal `json:"cost"`
	IsActive     *bool            `json:"is_active"`
}

// updateProductRequest es el body de PUT /api/v1/products/:id.
//
// Todos los campos son opcionales: solo se actualizan los punteros no nulos, de
// modo que un cliente puede enviar únicamente lo que cambia. name y unit, si
// vienen, no pueden quedar vacíos (lo valida el service después de recortar
// espacios).
type updateProductRequest struct {
	Name         *string          `json:"name" validate:"omitempty,max=150"`
	SKU          *string          `json:"sku" validate:"omitempty,max=50"`
	Description  *string          `json:"description" validate:"omitempty,max=2000"`
	Unit         *string          `json:"unit" validate:"omitempty,max=20"`
	IsSold       *bool            `json:"is_sold"`
	IsPurchased  *bool            `json:"is_purchased"`
	IsProduced   *bool            `json:"is_produced"`
	TrackStock   *bool            `json:"track_stock"`
	CurrentStock *decimal.Decimal `json:"current_stock"`
	MinStock     *decimal.Decimal `json:"min_stock"`
	SalePrice    *decimal.Decimal `json:"sale_price"`
	Cost         *decimal.Decimal `json:"cost"`
	IsActive     *bool            `json:"is_active"`
}

// productResponse es la representación pública de un producto.
//
// Los importes se devuelven como números JSON (json.Number) con los dígitos
// exactos que guarda la base: no se convierten a float64, así no se pierde
// precisión en el camino.
type productResponse struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	SKU          string      `json:"sku,omitempty"`
	Description  string      `json:"description,omitempty"`
	Unit         string      `json:"unit"`
	IsSold       bool        `json:"is_sold"`
	IsPurchased  bool        `json:"is_purchased"`
	IsProduced   bool        `json:"is_produced"`
	TrackStock   bool        `json:"track_stock"`
	CurrentStock json.Number `json:"current_stock"`
	MinStock     json.Number `json:"min_stock"`
	SalePrice    json.Number `json:"sale_price"`
	Cost         json.Number `json:"cost"`
	IsActive     bool        `json:"is_active"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// newProductResponse mapea la entidad de dominio al DTO de salida. Las fechas se
// normalizan a UTC, igual que en las respuestas de usuarios y terceros.
func newProductResponse(product *model.Product) productResponse {
	return productResponse{
		ID:           product.ID.String(),
		Name:         product.Name,
		SKU:          product.SKU,
		Description:  product.Description,
		Unit:         product.Unit,
		IsSold:       product.IsSold,
		IsPurchased:  product.IsPurchased,
		IsProduced:   product.IsProduced,
		TrackStock:   product.TrackStock,
		CurrentStock: json.Number(product.CurrentStock.String()),
		MinStock:     json.Number(product.MinStock.String()),
		SalePrice:    json.Number(product.SalePrice.String()),
		Cost:         json.Number(product.Cost.String()),
		IsActive:     product.IsActive,
		CreatedAt:    product.CreatedAt.UTC(),
		UpdatedAt:    product.UpdatedAt.UTC(),
	}
}

// newProductResponses mapea una lista de productos al DTO de salida. Devuelve un
// slice vacío (no nil) para que la API responda [] y no null.
func newProductResponses(products []model.Product) []productResponse {
	responses := make([]productResponse, 0, len(products))
	for i := range products {
		responses = append(responses, newProductResponse(&products[i]))
	}
	return responses
}
