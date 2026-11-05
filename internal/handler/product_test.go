package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/handler"
	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
	"github.com/pan001/backend/internal/service"
)

// memProductRepository es un repositorio de productos en memoria para no
// depender de PostgreSQL.
type memProductRepository struct {
	byID map[uuid.UUID]*model.Product
}

func newMemProductRepository() *memProductRepository {
	return &memProductRepository{byID: map[uuid.UUID]*model.Product{}}
}

func (r *memProductRepository) Create(_ context.Context, product *model.Product) error {
	if product.ID == uuid.Nil {
		product.ID = uuid.New()
	}
	now := time.Now().UTC()
	product.CreatedAt, product.UpdatedAt = now, now
	r.byID[product.ID] = product
	return nil
}

func (r *memProductRepository) FindByID(_ context.Context, id uuid.UUID) (*model.Product, error) {
	product, ok := r.byID[id]
	if !ok || product.DeletedAt.Valid {
		return nil, model.ErrProductNotFound
	}
	return product, nil
}

func (r *memProductRepository) List(_ context.Context, filters repository.ProductFilters) ([]model.Product, error) {
	search := strings.ToLower(strings.TrimSpace(filters.Search))
	products := make([]model.Product, 0, len(r.byID))
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

func (r *memProductRepository) Update(_ context.Context, product *model.Product) error {
	if _, ok := r.byID[product.ID]; !ok {
		return model.ErrProductNotFound
	}
	product.UpdatedAt = time.Now().UTC()
	r.byID[product.ID] = product
	return nil
}

func (r *memProductRepository) Delete(_ context.Context, id uuid.UUID) error {
	product, ok := r.byID[id]
	if !ok || product.DeletedAt.Valid {
		return model.ErrProductNotFound
	}
	product.DeletedAt = gorm.DeletedAt{Time: time.Now().UTC(), Valid: true}
	return nil
}

// memProductRepository implementa repository.ProductRepository.
var _ repository.ProductRepository = (*memProductRepository)(nil)

// newProductTestApp monta las rutas de productos sin middleware JWT: la
// protección del grupo se valida en internal/router/router_test.go.
func newProductTestApp(repo *memProductRepository) *fiber.App {
	productHandler := handler.NewProductHandler(service.NewProductService(repo))

	app := fiber.New()
	app.Post("/products", productHandler.Create)
	app.Get("/products", productHandler.List)
	app.Get("/products/:id", productHandler.GetByID)
	app.Put("/products/:id", productHandler.Update)
	app.Delete("/products/:id", productHandler.Delete)
	return app
}

// doProduct hace la petición y devuelve status + body.
func doProduct(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("app.Test() devolvió un error: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("no se pudo leer el body: %v", err)
	}

	return resp.StatusCode, string(raw)
}

// jsonBody serializa el payload de la petición para enviarlo por HTTP.
func jsonBody(t *testing.T, payload map[string]any) string {
	t.Helper()

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("no se pudo serializar el payload: %v", err)
	}
	return string(raw)
}

// errorMessage devuelve el mensaje de error de la respuesta. La app de test no
// registra el errorHandler JSON de main, así que Fiber responde el texto plano
// (igual que en contact_test.go); en producción main lo envuelve en
// {"error": "..."}.
func errorMessage(t *testing.T, body string) string {
	t.Helper()

	message := strings.TrimSpace(body)
	if message == "" {
		t.Fatal("body vacío, se esperaba un mensaje de error")
	}
	return message
}

// productPayload refleja la respuesta pública de un producto. Los decimales se
// deserializan como float64 para comprobar que la API los devuelve como números
// JSON (y no como cadenas).
type productPayload struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SKU          string    `json:"sku"`
	Description  string    `json:"description"`
	Unit         string    `json:"unit"`
	IsSold       bool      `json:"is_sold"`
	IsPurchased  bool      `json:"is_purchased"`
	IsProduced   bool      `json:"is_produced"`
	TrackStock   bool      `json:"track_stock"`
	CurrentStock float64   `json:"current_stock"`
	MinStock     float64   `json:"min_stock"`
	SalePrice    float64   `json:"sale_price"`
	Cost         float64   `json:"cost"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// decodeProduct deserializa una respuesta de un producto.
func decodeProduct(t *testing.T, body string) productPayload {
	t.Helper()

	var product productPayload
	if err := json.Unmarshal([]byte(body), &product); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	return product
}

// createProduct crea un producto por HTTP y devuelve su representación.
func createProduct(t *testing.T, app *fiber.App, payload map[string]any) productPayload {
	t.Helper()

	status, body := doProduct(t, app, http.MethodPost, "/products", jsonBody(t, payload))
	if status != fiber.StatusCreated {
		t.Fatalf("crear: status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}
	return decodeProduct(t, body)
}

// listProducts lista productos por HTTP aplicando la query string indicada.
func listProducts(t *testing.T, app *fiber.App, query string) []productPayload {
	t.Helper()

	status, body := doProduct(t, app, http.MethodGet, "/products"+query, "")
	if status != fiber.StatusOK {
		t.Fatalf("listar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var products []productPayload
	if err := json.Unmarshal([]byte(body), &products); err != nil {
		t.Fatalf("listar: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	return products
}

func TestCreateProductResponde201(t *testing.T) {
	app := newProductTestApp(newMemProductRepository())

	product := createProduct(t, app, map[string]any{
		"name":          "Pan francés",
		"sku":           "PAN-001",
		"description":   "Pan francés clásico",
		"unit":          "unidad",
		"is_sold":       true,
		"is_purchased":  false,
		"is_produced":   true,
		"track_stock":   true,
		"current_stock": 0,
		"min_stock":     20,
		"sale_price":    500,
		"cost":          180,
	})

	if _, err := uuid.Parse(product.ID); err != nil {
		t.Errorf("id = %q, se esperaba un UUID válido", product.ID)
	}
	if product.Name != "Pan francés" || product.SKU != "PAN-001" || product.Unit != "unidad" {
		t.Errorf("name/sku/unit = %q/%q/%q, se esperaban los datos enviados", product.Name, product.SKU, product.Unit)
	}
	if product.Description != "Pan francés clásico" {
		t.Errorf("description = %q, se esperaba la enviada", product.Description)
	}
	if !product.IsSold || product.IsPurchased || !product.IsProduced {
		t.Errorf("flags = %v/%v/%v, se esperaba true/false/true",
			product.IsSold, product.IsPurchased, product.IsProduced)
	}
	if !product.TrackStock || !product.IsActive {
		t.Errorf("track_stock/is_active = %v/%v, se esperaban true/true", product.TrackStock, product.IsActive)
	}
	if product.CurrentStock != 0 || product.MinStock != 20 || product.SalePrice != 500 || product.Cost != 180 {
		t.Errorf("decimales = %v/%v/%v/%v, se esperaban 0/20/500/180",
			product.CurrentStock, product.MinStock, product.SalePrice, product.Cost)
	}
	if product.CreatedAt.IsZero() || product.UpdatedAt.IsZero() {
		t.Error("created_at/updated_at no deberían venir vacíos")
	}
	// Los decimales viajan como números JSON, no como cadenas: si fueran
	// cadenas, json.Unmarshal habría fallado al llenar los float64.
}

func TestCreateProductValoresPorDefecto(t *testing.T) {
	app := newProductTestApp(newMemProductRepository())

	product := createProduct(t, app, map[string]any{
		"name": "  Pan francés  ",
		"unit": " unidad ",
	})

	if product.Name != "Pan francés" || product.Unit != "unidad" {
		t.Errorf("name/unit = %q/%q, se esperaban los valores sin espacios", product.Name, product.Unit)
	}
	if product.IsSold || product.IsPurchased || product.IsProduced {
		t.Errorf("flags = %v/%v/%v, se esperaban todos en false",
			product.IsSold, product.IsPurchased, product.IsProduced)
	}
	if !product.TrackStock || !product.IsActive {
		t.Errorf("track_stock/is_active = %v/%v, se esperaban true/true", product.TrackStock, product.IsActive)
	}
	if product.CurrentStock != 0 || product.MinStock != 0 || product.SalePrice != 0 || product.Cost != 0 {
		t.Errorf("decimales = %v/%v/%v/%v, se esperaban todos en cero",
			product.CurrentStock, product.MinStock, product.SalePrice, product.Cost)
	}
}

func TestCreateProductValidaCamposObligatorios(t *testing.T) {
	app := newProductTestApp(newMemProductRepository())

	casos := map[string]map[string]any{
		"sin nombre":             {"unit": "unidad"},
		"nombre en blanco":       {"name": "   ", "unit": "unidad"},
		"sin unidad":             {"name": "Pan francés"},
		"unidad en blanco":       {"name": "Pan francés", "unit": "  "},
		"unidad demasiado larga": {"name": "Pan", "unit": "un-super-unidad-de-medida"},
	}

	for name, payload := range casos {
		t.Run(name, func(t *testing.T) {
			status, body := doProduct(t, app, http.MethodPost, "/products", jsonBody(t, payload))
			if status != fiber.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
			}
			errorMessage(t, body)
		})
	}

	t.Run("json malformado", func(t *testing.T) {
		status, body := doProduct(t, app, http.MethodPost, "/products", "esto no es un json")
		if status != fiber.StatusBadRequest {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
		}
		errorMessage(t, body)
	})
}

func TestListProductsFiltra(t *testing.T) {
	app := newProductTestApp(newMemProductRepository())

	crear := func(payload map[string]any) string {
		t.Helper()
		return createProduct(t, app, payload).ID
	}

	crear(map[string]any{
		"name": "Pan francés", "sku": "PAN-001", "unit": "unidad",
		"is_sold": true, "is_produced": true,
	})
	crear(map[string]any{
		"name": "Harina 000", "sku": "HAR-000", "description": "Insumo para panificación",
		"unit": "kg", "is_purchased": true,
	})
	crear(map[string]any{
		"name": "Caja chica", "unit": "unidad",
		"is_purchased": true, "track_stock": false, "is_active": false,
	})

	casos := []struct {
		name  string
		query string
		count int
	}{
		{"sin filtros", "", 3},
		{"solo vendibles", "?is_sold=true", 1},
		{"no vendibles", "?is_sold=false", 2},
		{"solo producidos", "?is_produced=true", 1},
		{"comprados", "?is_purchased=true", 2},
		{"inactivos", "?is_active=false", 1},
		{"sin control de stock", "?track_stock=false", 1},
		{"filtros combinados", "?is_purchased=true&is_active=true", 1},
		{"búsqueda por nombre", "?search=har", 1},
		{"búsqueda por sku", "?search=PAN-001", 1},
		{"búsqueda por descripción", "?search=panificaci%C3%B3n", 1},
		{"sin coincidencias", "?search=no-existe", 0},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			products := listProducts(t, app, caso.query)
			if products == nil {
				t.Fatal("la respuesta debería ser un array JSON (aunque sea vacío), no null")
			}
			if len(products) != caso.count {
				t.Fatalf("len(products) = %d, se esperaban %d", len(products), caso.count)
			}
		})
	}

	t.Run("parámetro inválido", func(t *testing.T) {
		status, body := doProduct(t, app, http.MethodGet, "/products?is_sold=quizás", "")
		if status != fiber.StatusBadRequest {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
		}
		if message := errorMessage(t, body); !strings.Contains(message, "is_sold") {
			t.Errorf("mensaje = %q, debería mencionar el parámetro is_sold", message)
		}
	})
}

func TestGetProductPorID(t *testing.T) {
	app := newProductTestApp(newMemProductRepository())

	created := createProduct(t, app, map[string]any{
		"name": "Pan francés", "sku": "PAN-001", "unit": "unidad",
		"min_stock": 20, "sale_price": 500,
	})

	t.Run("existente", func(t *testing.T) {
		status, body := doProduct(t, app, http.MethodGet, "/products/"+created.ID, "")
		if status != fiber.StatusOK {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
		}
		product := decodeProduct(t, body)
		if product.ID != created.ID || product.Name != "Pan francés" {
			t.Errorf("producto = %+v, se esperaba el creado (%s)", product, created.ID)
		}
		if product.MinStock != 20 || product.SalePrice != 500 {
			t.Errorf("decimales = %v/%v, se esperaban 20/500", product.MinStock, product.SalePrice)
		}
	})

	t.Run("inexistente", func(t *testing.T) {
		status, body := doProduct(t, app, http.MethodGet, "/products/"+uuid.NewString(), "")
		if status != fiber.StatusNotFound {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
		}
		errorMessage(t, body)
	})

	t.Run("id inválido", func(t *testing.T) {
		status, body := doProduct(t, app, http.MethodGet, "/products/no-es-uuid", "")
		if status != fiber.StatusBadRequest {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
		}
		if message := errorMessage(t, body); !strings.Contains(message, "UUID") {
			t.Errorf("mensaje = %q, debería mencionar UUID", message)
		}
	})
}

func TestUpdateProductParcial(t *testing.T) {
	app := newProductTestApp(newMemProductRepository())

	created := createProduct(t, app, map[string]any{
		"name": "Pan francés", "sku": "PAN-001", "unit": "unidad",
		"current_stock": 10, "min_stock": 20, "sale_price": 500, "cost": 180,
		"is_sold": true,
	})

	status, body := doProduct(t, app, http.MethodPut, "/products/"+created.ID, jsonBody(t, map[string]any{
		"sale_price":   550.75,
		"min_stock":    25,
		"is_produced":  true,
		"is_purchased": true,
	}))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	updated := decodeProduct(t, body)
	if updated.SalePrice != 550.75 || updated.MinStock != 25 {
		t.Errorf("sale_price/min_stock = %v/%v, se esperaban 550.75/25", updated.SalePrice, updated.MinStock)
	}
	if !updated.IsProduced || !updated.IsPurchased {
		t.Errorf("is_produced/is_purchased = %v/%v, se esperaban true/true", updated.IsProduced, updated.IsPurchased)
	}
	// Los campos no enviados conservan su valor.
	if updated.Name != "Pan francés" || updated.SKU != "PAN-001" || updated.Unit != "unidad" {
		t.Errorf("campos no enviados modificados: %+v", updated)
	}
	if updated.CurrentStock != 10 || updated.Cost != 180 {
		t.Errorf("decimales no enviados modificados: %+v", updated)
	}
	if !updated.IsSold || !updated.TrackStock || !updated.IsActive {
		t.Errorf("flags no enviados modificados: %+v", updated)
	}

	// Lo actualizado quedó persistido.
	status, body = doProduct(t, app, http.MethodGet, "/products/"+created.ID, "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	reloaded := decodeProduct(t, body)
	if reloaded.SalePrice != 550.75 || reloaded.MinStock != 25 || !reloaded.IsProduced {
		t.Errorf("producto recargado = %+v, se esperaban los valores actualizados", reloaded)
	}
}

func TestUpdateProductValidaDatos(t *testing.T) {
	app := newProductTestApp(newMemProductRepository())

	created := createProduct(t, app, map[string]any{"name": "Pan francés", "unit": "unidad"})

	casos := map[string]map[string]any{
		"nombre vacío":       {"name": ""},
		"nombre en blanco":   {"name": "   "},
		"unidad vacía":       {"unit": ""},
		"unidad en blanco":   {"unit": "  "},
		"nombre muy extenso": {"name": strings.Repeat("a", 151)},
	}

	for name, payload := range casos {
		t.Run(name, func(t *testing.T) {
			status, body := doProduct(t, app, http.MethodPut, "/products/"+created.ID, jsonBody(t, payload))
			if status != fiber.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
			}
			errorMessage(t, body)
		})
	}

	t.Run("inexistente", func(t *testing.T) {
		status, body := doProduct(t, app, http.MethodPut, "/products/"+uuid.NewString(),
			jsonBody(t, map[string]any{"name": "Otro"}))
		if status != fiber.StatusNotFound {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
		}
		errorMessage(t, body)
	})

	t.Run("id inválido", func(t *testing.T) {
		status, body := doProduct(t, app, http.MethodPut, "/products/no-es-uuid",
			jsonBody(t, map[string]any{"name": "Otro"}))
		if status != fiber.StatusBadRequest {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
		}
		errorMessage(t, body)
	})

	// El producto original sigue intacto.
	status, body := doProduct(t, app, http.MethodGet, "/products/"+created.ID, "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	if product := decodeProduct(t, body); product.Name != "Pan francés" || product.Unit != "unidad" {
		t.Errorf("el producto original cambió: %+v", product)
	}
}

func TestDeleteProductResponde204(t *testing.T) {
	app := newProductTestApp(newMemProductRepository())

	created := createProduct(t, app, map[string]any{"name": "Pan francés", "unit": "unidad"})

	status, body := doProduct(t, app, http.MethodDelete, "/products/"+created.ID, "")
	if status != fiber.StatusNoContent {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusNoContent, body)
	}
	if strings.TrimSpace(body) != "" {
		t.Errorf("body = %q, se esperaba vacío", body)
	}

	// Ya no se puede obtener.
	status, body = doProduct(t, app, http.MethodGet, "/products/"+created.ID, "")
	if status != fiber.StatusNotFound {
		t.Fatalf("get tras borrar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}

	// Borrar dos veces también es 404.
	status, body = doProduct(t, app, http.MethodDelete, "/products/"+created.ID, "")
	if status != fiber.StatusNotFound {
		t.Fatalf("segundo delete: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}
	errorMessage(t, body)

	// Un id inválido es 400.
	status, body = doProduct(t, app, http.MethodDelete, "/products/no-es-uuid", "")
	if status != fiber.StatusBadRequest {
		t.Fatalf("id inválido: status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
	}
	errorMessage(t, body)
}
