package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

// fakeProductRepository es un repositorio en memoria para probar los casos de
// uso del catálogo sin base de datos.
type fakeProductRepository struct {
	byID        map[uuid.UUID]*model.Product
	lastFilters repository.ProductFilters
}

func newFakeProductRepository() *fakeProductRepository {
	return &fakeProductRepository{byID: map[uuid.UUID]*model.Product{}}
}

func (r *fakeProductRepository) Create(_ context.Context, product *model.Product) error {
	if product.ID == uuid.Nil {
		product.ID = uuid.New()
	}
	now := time.Now().UTC()
	product.CreatedAt, product.UpdatedAt = now, now
	r.byID[product.ID] = product
	return nil
}

func (r *fakeProductRepository) FindByID(_ context.Context, id uuid.UUID) (*model.Product, error) {
	product, ok := r.byID[id]
	if !ok || product.DeletedAt.Valid {
		return nil, model.ErrProductNotFound
	}
	return product, nil
}

func (r *fakeProductRepository) List(_ context.Context, filters repository.ProductFilters) ([]model.Product, error) {
	r.lastFilters = filters

	search := strings.ToLower(strings.TrimSpace(filters.Search))
	var products []model.Product
	for _, product := range r.byID {
		if product.DeletedAt.Valid {
			continue
		}
		if filters.IsSold != nil && product.IsSold != *filters.IsSold {
			continue
		}
		if filters.IsPurchased != nil && product.IsPurchased != *filters.IsPurchased {
			continue
		}
		if filters.IsProduced != nil && product.IsProduced != *filters.IsProduced {
			continue
		}
		if filters.IsActive != nil && product.IsActive != *filters.IsActive {
			continue
		}
		if filters.TrackStock != nil && product.TrackStock != *filters.TrackStock {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				product.Name, product.SKU, product.Description,
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		products = append(products, *product)
	}

	sort.Slice(products, func(i, j int) bool { return products[i].Name < products[j].Name })
	return products, nil
}

func (r *fakeProductRepository) Update(_ context.Context, product *model.Product) error {
	if _, ok := r.byID[product.ID]; !ok {
		return model.ErrProductNotFound
	}
	product.UpdatedAt = time.Now().UTC()
	r.byID[product.ID] = product
	return nil
}

func (r *fakeProductRepository) Delete(_ context.Context, id uuid.UUID) error {
	product, ok := r.byID[id]
	if !ok || product.DeletedAt.Valid {
		return model.ErrProductNotFound
	}
	product.DeletedAt = gorm.DeletedAt{Time: time.Now().UTC(), Valid: true}
	return nil
}

func TestCreateProductAplicaValoresPorDefecto(t *testing.T) {
	repo := newFakeProductRepository()
	svc := NewProductService(repo)

	product, err := svc.Create(context.Background(), CreateProductInput{
		Name:        "  Pan francés  ",
		SKU:         " PAN-001 ",
		Description: "  Pan francés clásico ",
		Unit:        " unidad ",
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	if product.Name != "Pan francés" || product.Unit != "unidad" {
		t.Errorf("Name/Unit = %q/%q, se esperaban los valores sin espacios", product.Name, product.Unit)
	}
	if product.SKU != "PAN-001" || product.Description != "Pan francés clásico" {
		t.Errorf("SKU/Description = %q/%q, se esperaban los valores sin espacios", product.SKU, product.Description)
	}
	if product.IsSold || product.IsPurchased || product.IsProduced {
		t.Errorf("flags = %v/%v/%v, se esperaban todos en false",
			product.IsSold, product.IsPurchased, product.IsProduced)
	}
	if !product.TrackStock {
		t.Error("TrackStock = false, se esperaba true por defecto")
	}
	if !product.IsActive {
		t.Error("IsActive = false, se esperaba true por defecto")
	}
	if !product.CurrentStock.IsZero() || !product.MinStock.IsZero() ||
		!product.SalePrice.IsZero() || !product.Cost.IsZero() {
		t.Errorf("decimales = %s/%s/%s/%s, se esperaban todos en cero",
			product.CurrentStock, product.MinStock, product.SalePrice, product.Cost)
	}
	if repo.byID[product.ID] != product {
		t.Error("el producto no quedó persistido en el repositorio")
	}
}

func TestCreateProductFlagsYDecimalesLibres(t *testing.T) {
	svc := NewProductService(newFakeProductRepository())

	// Los tres flags pueden combinarse y los decimales se guardan tal cual.
	product, err := svc.Create(context.Background(), CreateProductInput{
		Name:         "Harina 000",
		Unit:         "kg",
		IsSold:       boolPtr(true),
		IsPurchased:  boolPtr(true),
		IsProduced:   boolPtr(false),
		TrackStock:   boolPtr(false),
		CurrentStock: decimalPtr("12.500"),
		MinStock:     decimalPtr("20"),
		SalePrice:    decimalPtr("1500.50"),
		Cost:         decimalPtr("980.25"),
		IsActive:     boolPtr(false),
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	if !product.IsSold || !product.IsPurchased || product.IsProduced {
		t.Errorf("flags = %v/%v/%v, se esperaba true/true/false",
			product.IsSold, product.IsPurchased, product.IsProduced)
	}
	if product.TrackStock || product.IsActive {
		t.Errorf("TrackStock/IsActive = %v/%v, se esperaban false/false", product.TrackStock, product.IsActive)
	}

	esperados := map[string][2]decimal.Decimal{
		"current_stock": {product.CurrentStock, decimal.RequireFromString("12.5")},
		"min_stock":     {product.MinStock, decimal.RequireFromString("20")},
		"sale_price":    {product.SalePrice, decimal.RequireFromString("1500.5")},
		"cost":          {product.Cost, decimal.RequireFromString("980.25")},
	}
	for campo, par := range esperados {
		if !par[0].Equal(par[1]) {
			t.Errorf("%s = %s, se esperaba %s", campo, par[0], par[1])
		}
	}
}

func TestCreateProductValidaCamposObligatorios(t *testing.T) {
	svc := NewProductService(newFakeProductRepository())

	casos := map[string]struct {
		in   CreateProductInput
		want error
	}{
		"sin nombre":       {CreateProductInput{Unit: "unidad"}, model.ErrProductNameRequired},
		"nombre en blanco": {CreateProductInput{Name: "   ", Unit: "unidad"}, model.ErrProductNameRequired},
		"sin unidad":       {CreateProductInput{Name: "Pan francés"}, model.ErrProductUnitRequired},
		"unidad en blanco": {CreateProductInput{Name: "Pan francés", Unit: "  "}, model.ErrProductUnitRequired},
	}

	for name, caso := range casos {
		t.Run(name, func(t *testing.T) {
			_, err := svc.Create(context.Background(), caso.in)
			if !errors.Is(err, caso.want) {
				t.Fatalf("error = %v, se esperaba %v", err, caso.want)
			}
		})
	}
}

func TestListProductsFiltra(t *testing.T) {
	repo := newFakeProductRepository()
	svc := NewProductService(repo)

	crear := func(in CreateProductInput) {
		t.Helper()
		if _, err := svc.Create(context.Background(), in); err != nil {
			t.Fatalf("Create() devolvió un error inesperado: %v", err)
		}
	}

	crear(CreateProductInput{
		Name: "Pan francés", SKU: "PAN-001", Unit: "unidad",
		IsSold: boolPtr(true), IsProduced: boolPtr(true),
	})
	crear(CreateProductInput{
		Name: "Harina 000", SKU: "HAR-000", Description: "Insumo para panificación",
		Unit: "kg", IsPurchased: boolPtr(true),
	})
	crear(CreateProductInput{
		Name: "Caja chica", Unit: "unidad",
		IsPurchased: boolPtr(true), TrackStock: boolPtr(false), IsActive: boolPtr(false),
	})

	casos := []struct {
		name  string
		in    ListProductsInput
		count int
	}{
		{"sin filtros", ListProductsInput{}, 3},
		{"solo vendibles", ListProductsInput{IsSold: boolPtr(true)}, 1},
		{"solo producidos", ListProductsInput{IsProduced: boolPtr(true)}, 1},
		{"comprados", ListProductsInput{IsPurchased: boolPtr(true)}, 2},
		{"inactivos", ListProductsInput{IsActive: boolPtr(false)}, 1},
		{"sin control de stock", ListProductsInput{TrackStock: boolPtr(false)}, 1},
		{"búsqueda por nombre", ListProductsInput{Search: "har"}, 1},
		{"búsqueda por sku", ListProductsInput{Search: " pan-001 "}, 1},
		{"búsqueda por descripción", ListProductsInput{Search: "PANIFICACIÓN"}, 1},
		{"sin coincidencias", ListProductsInput{Search: "no-existe"}, 0},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			products, err := svc.List(context.Background(), caso.in)
			if err != nil {
				t.Fatalf("List() devolvió un error inesperado: %v", err)
			}
			if products == nil {
				t.Fatal("List() devolvió nil, se esperaba un slice (aunque sea vacío)")
			}
			if len(products) != caso.count {
				t.Fatalf("len(products) = %d, se esperaban %d", len(products), caso.count)
			}
		})
	}

	// El service recorta el texto de búsqueda antes de pasarlo al repositorio.
	if repo.lastFilters.Search != "no-existe" {
		t.Errorf("Search = %q, se esperaba el texto sin espacios", repo.lastFilters.Search)
	}
}

func TestUpdateProductParcial(t *testing.T) {
	repo := newFakeProductRepository()
	svc := NewProductService(repo)

	created, err := svc.Create(context.Background(), CreateProductInput{
		Name:         "Pan francés",
		SKU:          "PAN-001",
		Unit:         "unidad",
		CurrentStock: decimalPtr("10"),
		MinStock:     decimalPtr("20"),
		SalePrice:    decimalPtr("500"),
		Cost:         decimalPtr("180"),
		IsSold:       boolPtr(true),
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	updated, err := svc.Update(context.Background(), created.ID, UpdateProductInput{
		SalePrice:   decimalPtr("550.75"),
		MinStock:    decimalPtr("25"),
		IsProduced:  boolPtr(true),
		IsPurchased: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("Update() devolvió un error inesperado: %v", err)
	}

	if !updated.SalePrice.Equal(decimal.RequireFromString("550.75")) {
		t.Errorf("SalePrice = %s, se esperaba 550.75", updated.SalePrice)
	}
	if !updated.MinStock.Equal(decimal.RequireFromString("25")) {
		t.Errorf("MinStock = %s, se esperaba 25", updated.MinStock)
	}
	if !updated.IsProduced || !updated.IsPurchased {
		t.Errorf("IsProduced/IsPurchased = %v/%v, se esperaban true/true",
			updated.IsProduced, updated.IsPurchased)
	}
	// Los campos no enviados conservan su valor.
	if updated.Name != "Pan francés" || updated.Unit != "unidad" || updated.SKU != "PAN-001" {
		t.Errorf("campos no enviados modificados: %+v", updated)
	}
	if !updated.IsSold || !updated.TrackStock || !updated.IsActive {
		t.Errorf("flags no enviados modificados: %+v", updated)
	}
	if !updated.CurrentStock.Equal(decimal.RequireFromString("10")) ||
		!updated.Cost.Equal(decimal.RequireFromString("180")) {
		t.Errorf("decimales no enviados modificados: %+v", updated)
	}
}

func TestUpdateProductValidaNombreYUnidad(t *testing.T) {
	repo := newFakeProductRepository()
	svc := NewProductService(repo)

	created, err := svc.Create(context.Background(), CreateProductInput{Name: "Pan francés", Unit: "unidad"})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	casos := []struct {
		name string
		in   UpdateProductInput
		want error
	}{
		{"nombre vacío", UpdateProductInput{Name: stringPtr("")}, model.ErrProductNameRequired},
		{"nombre en blanco", UpdateProductInput{Name: stringPtr("   ")}, model.ErrProductNameRequired},
		{"unidad vacía", UpdateProductInput{Unit: stringPtr("")}, model.ErrProductUnitRequired},
		{"unidad en blanco", UpdateProductInput{Unit: stringPtr("  ")}, model.ErrProductUnitRequired},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			_, err := svc.Update(context.Background(), created.ID, caso.in)
			if !errors.Is(err, caso.want) {
				t.Fatalf("error = %v, se esperaba %v", err, caso.want)
			}
		})
	}

	if repo.byID[created.ID].Name != "Pan francés" || repo.byID[created.ID].Unit != "unidad" {
		t.Error("los valores guardados no deberían haber cambiado")
	}
}

func TestUpdateProductNoEncontrado(t *testing.T) {
	svc := NewProductService(newFakeProductRepository())

	_, err := svc.Update(context.Background(), uuid.New(), UpdateProductInput{Name: stringPtr("Nuevo")})
	if !errors.Is(err, model.ErrProductNotFound) {
		t.Fatalf("error = %v, se esperaba model.ErrProductNotFound", err)
	}
}

func TestDeleteProductHaceSoftDelete(t *testing.T) {
	repo := newFakeProductRepository()
	svc := NewProductService(repo)

	created, err := svc.Create(context.Background(), CreateProductInput{Name: "Pan francés", Unit: "unidad"})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	if err := svc.Delete(context.Background(), created.ID); err != nil {
		t.Fatalf("Delete() devolvió un error inesperado: %v", err)
	}
	if !repo.byID[created.ID].DeletedAt.Valid {
		t.Error("DeletedAt no quedó marcado: no fue un soft delete")
	}

	// El registro sigue en la tabla, pero ya no se devuelve.
	if _, err := svc.GetByID(context.Background(), created.ID); !errors.Is(err, model.ErrProductNotFound) {
		t.Errorf("GetByID() después del delete = %v, se esperaba model.ErrProductNotFound", err)
	}

	// Borrar dos veces también es 404.
	if err := svc.Delete(context.Background(), created.ID); !errors.Is(err, model.ErrProductNotFound) {
		t.Errorf("segundo Delete() = %v, se esperaba model.ErrProductNotFound", err)
	}
}

// fakeProductRepository implementa repository.ProductRepository.
var _ repository.ProductRepository = (*fakeProductRepository)(nil)

func decimalPtr(value string) *decimal.Decimal {
	d := decimal.RequireFromString(value)
	return &d
}
