package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

// Valores por defecto del dominio de productos.
const (
	// DefaultProductIsSold, DefaultProductIsPurchased y DefaultProductIsProduced
	// nacen en false: el ítem se habilita para cada uso según el negocio. Los
	// tres flags pueden combinarse libremente.
	DefaultProductIsSold      = false
	DefaultProductIsPurchased = false
	DefaultProductIsProduced  = false
	// DefaultProductTrackStock es true: lo habitual es controlar el stock.
	DefaultProductTrackStock = true
	// DefaultProductIsActive es true: los productos nacen activos.
	DefaultProductIsActive = true
)

// CreateProductInput son los datos para dar de alta un producto.
//
// Los booleanos y los decimales son punteros para distinguir "no enviado" (nil)
// de un false o 0 explícitos: el service resuelve los valores por defecto
// (track_stock=true, is_active=true, el resto false y los importes en 0).
type CreateProductInput struct {
	Name         string
	SKU          string
	Description  string
	Unit         string
	IsSold       *bool
	IsPurchased  *bool
	IsProduced   *bool
	TrackStock   *bool
	CurrentStock *decimal.Decimal
	MinStock     *decimal.Decimal
	SalePrice    *decimal.Decimal
	Cost         *decimal.Decimal
	IsActive     *bool
}

// UpdateProductInput son los campos a modificar de un producto. Un puntero en
// nil significa "no tocar": el endpoint PUT actualiza solo lo que se envía.
type UpdateProductInput struct {
	Name         *string
	SKU          *string
	Description  *string
	Unit         *string
	IsSold       *bool
	IsPurchased  *bool
	IsProduced   *bool
	TrackStock   *bool
	CurrentStock *decimal.Decimal
	MinStock     *decimal.Decimal
	SalePrice    *decimal.Decimal
	Cost         *decimal.Decimal
	IsActive     *bool
}

// ListProductsInput son los filtros opcionales de listado. Los punteros en nil
// no filtran; Search busca en nombre, SKU y descripción.
type ListProductsInput struct {
	IsSold      *bool
	IsPurchased *bool
	IsProduced  *bool
	IsActive    *bool
	TrackStock  *bool
	Search      string
}

// ProductService agrupa los casos de uso del catálogo de productos.
type ProductService struct {
	products repository.ProductRepository
}

// NewProductService construye el servicio de productos.
func NewProductService(products repository.ProductRepository) *ProductService {
	return &ProductService{products: products}
}

// Create da de alta un producto.
//
// Reglas:
//   - Name y Unit son obligatorios; se guardan sin espacios sobrantes.
//   - El resto es opcional: SKU y descripción se recortan, los flags pueden ser
//     false, true o cualquier combinación y los importes no enviados quedan en
//     cero.
//   - track_stock e is_active son true salvo que se indique lo contrario.
func (s *ProductService) Create(ctx context.Context, in CreateProductInput) (*model.Product, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, model.ErrProductNameRequired
	}
	unit := strings.TrimSpace(in.Unit)
	if unit == "" {
		return nil, model.ErrProductUnitRequired
	}

	product := &model.Product{
		Name:         name,
		SKU:          strings.TrimSpace(in.SKU),
		Description:  strings.TrimSpace(in.Description),
		Unit:         unit,
		IsSold:       boolOr(in.IsSold, DefaultProductIsSold),
		IsPurchased:  boolOr(in.IsPurchased, DefaultProductIsPurchased),
		IsProduced:   boolOr(in.IsProduced, DefaultProductIsProduced),
		TrackStock:   boolOr(in.TrackStock, DefaultProductTrackStock),
		CurrentStock: decimalOr(in.CurrentStock),
		MinStock:     decimalOr(in.MinStock),
		SalePrice:    decimalOr(in.SalePrice),
		Cost:         decimalOr(in.Cost),
		IsActive:     boolOr(in.IsActive, DefaultProductIsActive),
	}

	if err := s.products.Create(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

// GetByID devuelve un producto por su identificador.
//
// Reglas:
//   - Si no existe o fue soft-deleted → model.ErrProductNotFound.
func (s *ProductService) GetByID(ctx context.Context, id uuid.UUID) (*model.Product, error) {
	if id == uuid.Nil {
		return nil, model.ErrProductNotFound
	}

	return s.products.FindByID(ctx, id)
}

// List devuelve los productos no eliminados con los filtros indicados. El
// resultado nunca es nil, así la API responde [] y no null.
func (s *ProductService) List(ctx context.Context, in ListProductsInput) ([]model.Product, error) {
	products, err := s.products.List(ctx, repository.ProductFilters{
		IsSold:      in.IsSold,
		IsPurchased: in.IsPurchased,
		IsProduced:  in.IsProduced,
		IsActive:    in.IsActive,
		TrackStock:  in.TrackStock,
		Search:      strings.TrimSpace(in.Search),
	})
	if err != nil {
		return nil, err
	}
	if products == nil {
		return []model.Product{}, nil
	}
	return products, nil
}

// Update modifica solo los campos enviados (los nil se dejan como estaban).
//
// Reglas:
//   - Si el producto no existe → model.ErrProductNotFound.
//   - Si se envía name o unit, no pueden quedar vacíos (después de recortar
//     espacios) → model.ErrProductNameRequired / model.ErrProductUnitRequired.
func (s *ProductService) Update(ctx context.Context, id uuid.UUID, in UpdateProductInput) (*model.Product, error) {
	product, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, model.ErrProductNameRequired
		}
		product.Name = name
	}
	if in.SKU != nil {
		product.SKU = strings.TrimSpace(*in.SKU)
	}
	if in.Description != nil {
		product.Description = strings.TrimSpace(*in.Description)
	}
	if in.Unit != nil {
		unit := strings.TrimSpace(*in.Unit)
		if unit == "" {
			return nil, model.ErrProductUnitRequired
		}
		product.Unit = unit
	}
	if in.IsSold != nil {
		product.IsSold = *in.IsSold
	}
	if in.IsPurchased != nil {
		product.IsPurchased = *in.IsPurchased
	}
	if in.IsProduced != nil {
		product.IsProduced = *in.IsProduced
	}
	if in.TrackStock != nil {
		product.TrackStock = *in.TrackStock
	}
	if in.CurrentStock != nil {
		product.CurrentStock = *in.CurrentStock
	}
	if in.MinStock != nil {
		product.MinStock = *in.MinStock
	}
	if in.SalePrice != nil {
		product.SalePrice = *in.SalePrice
	}
	if in.Cost != nil {
		product.Cost = *in.Cost
	}
	if in.IsActive != nil {
		product.IsActive = *in.IsActive
	}

	if err := s.products.Update(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

// Delete hace un soft delete del producto.
//
// Reglas:
//   - Si no existe o ya estaba eliminado → model.ErrProductNotFound.
func (s *ProductService) Delete(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return model.ErrProductNotFound
	}

	return s.products.Delete(ctx, id)
}

// decimalOr devuelve el valor apuntado o cero cuando el campo no se envió.
func decimalOr(value *decimal.Decimal) decimal.Decimal {
	if value == nil {
		return decimal.Zero
	}
	return *value
}
