package router_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/handler"
	"github.com/pan001/backend/internal/infrastructure/auth"
	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
	"github.com/pan001/backend/internal/router"
	"github.com/pan001/backend/internal/service"
)

const testSecret = "secreto-de-pruebas"

// memUserRepository es un repositorio en memoria para montar el router completo
// sin depender de PostgreSQL.
type memUserRepository struct {
	byEmail map[string]*model.User
	byID    map[uuid.UUID]*model.User
}

func newMemUserRepository() *memUserRepository {
	return &memUserRepository{
		byEmail: map[string]*model.User{},
		byID:    map[uuid.UUID]*model.User{},
	}
}

func (r *memUserRepository) Create(_ context.Context, user *model.User) error {
	if _, exists := r.byEmail[user.Email]; exists {
		return model.ErrEmailAlreadyExists
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now().UTC()
	user.CreatedAt, user.UpdatedAt = now, now
	r.byEmail[user.Email] = user
	r.byID[user.ID] = user
	return nil
}

func (r *memUserRepository) FindByEmail(_ context.Context, email string) (*model.User, error) {
	user, ok := r.byEmail[email]
	if !ok || user.DeletedAt.Valid {
		return nil, model.ErrUserNotFound
	}
	return user, nil
}

func (r *memUserRepository) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	user, ok := r.byID[id]
	if !ok || user.DeletedAt.Valid {
		return nil, model.ErrUserNotFound
	}
	return user, nil
}

func (r *memUserRepository) UpdateLastLogin(_ context.Context, id uuid.UUID, loggedAt time.Time) error {
	user, ok := r.byID[id]
	if !ok {
		return model.ErrUserNotFound
	}
	user.LastLoginAt = &loggedAt
	return nil
}

func (r *memUserRepository) Update(_ context.Context, user *model.User) error {
	stored, ok := r.byID[user.ID]
	if !ok {
		return model.ErrUserNotFound
	}
	if existing, taken := r.byEmail[user.Email]; taken && existing.ID != user.ID {
		return model.ErrEmailAlreadyExists
	}
	// El email es la clave del índice: si cambió, hay que reindexar.
	if stored.Email != user.Email {
		delete(r.byEmail, stored.Email)
	}
	user.UpdatedAt = time.Now().UTC()
	r.byEmail[user.Email] = user
	r.byID[user.ID] = user
	return nil
}

func (r *memUserRepository) UpdatePassword(_ context.Context, id uuid.UUID, passwordHash string) error {
	user, ok := r.byID[id]
	if !ok {
		return model.ErrUserNotFound
	}
	user.PasswordHash = passwordHash
	return nil
}

// newTestApp monta las rutas reales con el middleware JWT real y un
// repositorio en memoria.
func newTestApp(t *testing.T) (*fiber.App, *memUserRepository) {
	t.Helper()

	repo := newMemUserRepository()
	tokens := auth.NewJWTManager(testSecret, auth.DefaultTokenTTL)
	authHandler := handler.NewAuthHandler(service.NewAuthService(repo, tokens))

	contactRepo := newMemContactRepository()
	contactHandler := handler.NewContactHandler(service.NewContactService(contactRepo))

	productRepo := newMemProductRepository()
	productHandler := handler.NewProductHandler(service.NewProductService(productRepo))

	app := fiber.New()
	router.Register(app, router.Handlers{
		Auth:     authHandler,
		Contacts: contactHandler,
		Products: productHandler,
	}, tokens)

	return app, repo
}

// memContactRepository es un repositorio de terceros en memoria para montar el
// router completo sin depender de PostgreSQL.
type memContactRepository struct {
	byID map[uuid.UUID]*model.Contact
}

func newMemContactRepository() *memContactRepository {
	return &memContactRepository{byID: map[uuid.UUID]*model.Contact{}}
}

func (r *memContactRepository) Create(_ context.Context, contact *model.Contact) error {
	if contact.ID == uuid.Nil {
		contact.ID = uuid.New()
	}
	now := time.Now().UTC()
	contact.CreatedAt, contact.UpdatedAt = now, now
	r.byID[contact.ID] = contact
	return nil
}

func (r *memContactRepository) FindByID(_ context.Context, id uuid.UUID) (*model.Contact, error) {
	contact, ok := r.byID[id]
	if !ok || contact.DeletedAt.Valid {
		return nil, model.ErrContactNotFound
	}
	return contact, nil
}

func (r *memContactRepository) List(_ context.Context, filters repository.ContactFilters) ([]model.Contact, error) {
	search := strings.ToLower(strings.TrimSpace(filters.Search))
	contacts := make([]model.Contact, 0, len(r.byID))
	for _, contact := range r.byID {
		if contact.DeletedAt.Valid {
			continue
		}
		if filters.IsClient != nil && contact.IsClient != *filters.IsClient {
			continue
		}
		if filters.IsSupplier != nil && contact.IsSupplier != *filters.IsSupplier {
			continue
		}
		if filters.IsActive != nil && contact.IsActive != *filters.IsActive {
			continue
		}
		if search != "" {
			haystack := strings.ToLower(strings.Join([]string{
				contact.Name, contact.BusinessName, contact.TaxID,
				contact.Email, contact.Phone, contact.WhatsApp,
			}, " "))
			if !strings.Contains(haystack, search) {
				continue
			}
		}
		contacts = append(contacts, *contact)
	}
	return contacts, nil
}

func (r *memContactRepository) Update(_ context.Context, contact *model.Contact) error {
	if _, ok := r.byID[contact.ID]; !ok {
		return model.ErrContactNotFound
	}
	contact.UpdatedAt = time.Now().UTC()
	r.byID[contact.ID] = contact
	return nil
}

func (r *memContactRepository) Delete(_ context.Context, id uuid.UUID) error {
	contact, ok := r.byID[id]
	if !ok || contact.DeletedAt.Valid {
		return model.ErrContactNotFound
	}
	contact.DeletedAt = gorm.DeletedAt{Time: time.Now().UTC(), Valid: true}
	return nil
}

// memContactRepository implementa repository.ContactRepository.
var _ repository.ContactRepository = (*memContactRepository)(nil)

// memProductRepository es un repositorio de productos en memoria para montar el
// router completo sin depender de PostgreSQL.
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

// do hace la petición y devuelve status + body.
func do(t *testing.T, app *fiber.App, method, path, body, token string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	if token != "" {
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
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

// register crea un usuario y devuelve el token emitido en la respuesta.
func register(t *testing.T, app *fiber.App, email, role string) string {
	t.Helper()

	body := `{"email":"` + email + `","password":"secreto123","full_name":"María Pérez","role":"` + role + `"}`
	status, responseBody := do(t, app, http.MethodPost, "/api/v1/auth/register", body, "")
	if status != fiber.StatusCreated {
		t.Fatalf("register: status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, responseBody)
	}

	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatalf("register: respuesta no es JSON válido: %v (body: %s)", err, responseBody)
	}
	if response.Token == "" {
		t.Fatal("register: la respuesta no incluye token")
	}

	return response.Token
}

func TestRegisterYLoginSonPublicos(t *testing.T) {
	app, _ := newTestApp(t)

	// Register sin Authorization: debe seguir siendo público.
	status, body := do(t, app, http.MethodPost, "/api/v1/auth/register",
		`{"email":"maria@pan001.com","password":"secreto123","full_name":"María Pérez","role":"cajero"}`, "")
	if status != fiber.StatusCreated {
		t.Fatalf("register: status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}

	// Login sin Authorization: debe seguir siendo público.
	status, body = do(t, app, http.MethodPost, "/api/v1/auth/login",
		`{"email":"maria@pan001.com","password":"secreto123"}`, "")
	if status != fiber.StatusOK {
		t.Fatalf("login: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var response struct {
		Token string `json:"token"`
		User  struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("login: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if response.Token == "" || response.User.Email != "maria@pan001.com" {
		t.Errorf("login: respuesta inesperada: %+v", response)
	}
}

func TestMeSinTokenResponde401(t *testing.T) {
	app, _ := newTestApp(t)

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", "")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}

	var response struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if response.Error != "falta el header Authorization" {
		t.Errorf("error = %q, se esperaba el mensaje de header faltante", response.Error)
	}
}

func TestMeConTokenValidoDevuelveUsuario(t *testing.T) {
	app, _ := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleCajero)

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", token)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var response struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}

	if response.ID == "" || response.ID == uuid.Nil.String() {
		t.Errorf("id = %q, se esperaba el UUID del usuario", response.ID)
	}
	if response.Email != "maria@pan001.com" {
		t.Errorf("email = %q, se esperaba maria@pan001.com", response.Email)
	}
	if response.FullName != "María Pérez" {
		t.Errorf("full_name = %q, se esperaba María Pérez", response.FullName)
	}
	if response.Role != model.RoleCajero {
		t.Errorf("role = %q, se esperaba %q", response.Role, model.RoleCajero)
	}
	if !response.IsActive {
		t.Error("is_active = false, el usuario recién registrado debe estar activo")
	}
	if strings.Contains(body, "password") {
		t.Errorf("la respuesta no debe exponer la contraseña: %s", body)
	}
}

func TestMeTokenInvalidoResponde401(t *testing.T) {
	app, _ := newTestApp(t)

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", "no-es-un-jwt")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}

	var response struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if response.Error != "token inválido o expirado" {
		t.Errorf("error = %q, se esperaba \"token inválido o expirado\"", response.Error)
	}
}

func TestMeUsuarioEliminadoResponde401(t *testing.T) {
	app, repo := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	// El token sigue vigente, pero el usuario ya no existe.
	delete(repo.byEmail, "maria@pan001.com")
	repo.byID = map[uuid.UUID]*model.User{}

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", token)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
	if !strings.Contains(body, "el usuario del token ya no existe") {
		t.Errorf("body = %q, se esperaba el mensaje de usuario inexistente", body)
	}
}

func TestMeUsuarioInactivoResponde401(t *testing.T) {
	app, repo := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	// Un admin desactiva la cuenta después de emitir el token.
	repo.byEmail["maria@pan001.com"].IsActive = false

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", token)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
	if !strings.Contains(body, "el usuario está desactivado") {
		t.Errorf("body = %q, se esperaba el mensaje de usuario desactivado", body)
	}
}

func TestContactsSinTokenResponde401(t *testing.T) {
	app, _ := newTestApp(t)
	path := "/api/v1/contacts/" + uuid.New().String()

	casos := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/contacts", `{"name":"Panadería El Sol"}`},
		{http.MethodGet, "/api/v1/contacts", ""},
		{http.MethodGet, path, ""},
		{http.MethodPut, path, `{"name":"Otro Nombre"}`},
		{http.MethodDelete, path, ""},
	}

	for _, caso := range casos {
		t.Run(caso.method+" "+caso.path, func(t *testing.T) {
			status, body := do(t, app, caso.method, caso.path, caso.body, "")
			if status != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
			}
			if !strings.Contains(body, "falta el header Authorization") {
				t.Errorf("body = %q, se esperaba el mensaje de header faltante", body)
			}
		})
	}
}

func TestContactsCRUDConToken(t *testing.T) {
	app, _ := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	// POST /api/v1/contacts
	status, body := do(t, app, http.MethodPost, "/api/v1/contacts", `{
		"name": "Panadería El Sol",
		"business_name": "El Sol SRL",
		"tax_id": "30-12345678-9",
		"email": "contacto@elsol.com",
		"phone": "1122334455",
		"is_client": true,
		"is_supplier": false
	}`, token)
	if status != fiber.StatusCreated {
		t.Fatalf("crear: status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}

	var created struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		IsClient bool   `json:"is_client"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("crear: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if created.ID == "" || created.Name != "Panadería El Sol" || !created.IsClient || !created.IsActive {
		t.Fatalf("crear: respuesta inesperada: %+v", created)
	}

	// GET /api/v1/contacts con filtros
	status, body = do(t, app, http.MethodGet, "/api/v1/contacts?is_client=true&search=sol", "", token)
	if status != fiber.StatusOK {
		t.Fatalf("listar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	var contacts []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &contacts); err != nil {
		t.Fatalf("listar: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if len(contacts) != 1 || contacts[0].ID != created.ID {
		t.Fatalf("listar: contacts = %+v, se esperaba solo el tercero creado", contacts)
	}

	// GET /api/v1/contacts/:id
	status, body = do(t, app, http.MethodGet, "/api/v1/contacts/"+created.ID, "", token)
	if status != fiber.StatusOK {
		t.Fatalf("obtener: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	// PUT /api/v1/contacts/:id (solo cambia el teléfono)
	status, body = do(t, app, http.MethodPut, "/api/v1/contacts/"+created.ID,
		`{"phone":"1199998888"}`, token)
	if status != fiber.StatusOK {
		t.Fatalf("actualizar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	var updated struct {
		Name     string `json:"name"`
		Phone    string `json:"phone"`
		IsClient bool   `json:"is_client"`
	}
	if err := json.Unmarshal([]byte(body), &updated); err != nil {
		t.Fatalf("actualizar: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if updated.Phone != "1199998888" || updated.Name != "Panadería El Sol" || !updated.IsClient {
		t.Errorf("actualizar: respuesta inesperada: %+v", updated)
	}

	// DELETE /api/v1/contacts/:id (soft delete)
	status, body = do(t, app, http.MethodDelete, "/api/v1/contacts/"+created.ID, "", token)
	if status != fiber.StatusNoContent {
		t.Fatalf("borrar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNoContent, body)
	}

	// El tercero ya no existe para la API.
	status, body = do(t, app, http.MethodGet, "/api/v1/contacts/"+created.ID, "", token)
	if status != fiber.StatusNotFound {
		t.Fatalf("obtener tras borrar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}
}

func TestProductsSinTokenResponde401(t *testing.T) {
	app, _ := newTestApp(t)
	path := "/api/v1/products/" + uuid.New().String()

	casos := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/v1/products", `{"name":"Pan francés","unit":"unidad"}`},
		{http.MethodGet, "/api/v1/products", ""},
		{http.MethodGet, path, ""},
		{http.MethodPut, path, `{"name":"Otro Nombre"}`},
		{http.MethodDelete, path, ""},
	}

	for _, caso := range casos {
		t.Run(caso.method+" "+caso.path, func(t *testing.T) {
			status, body := do(t, app, caso.method, caso.path, caso.body, "")
			if status != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
			}
			if !strings.Contains(body, "falta el header Authorization") {
				t.Errorf("body = %q, se esperaba el mensaje de header faltante", body)
			}
		})
	}
}

func TestProductsCRUDConToken(t *testing.T) {
	app, _ := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	// POST /api/v1/products
	status, body := do(t, app, http.MethodPost, "/api/v1/products", `{
		"name": "Pan francés",
		"sku": "PAN-001",
		"description": "Pan francés clásico",
		"unit": "unidad",
		"is_sold": true,
		"is_purchased": false,
		"is_produced": true,
		"track_stock": true,
		"current_stock": 0,
		"min_stock": 20,
		"sale_price": 500,
		"cost": 180
	}`, token)
	if status != fiber.StatusCreated {
		t.Fatalf("crear: status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}

	var created struct {
		ID         string  `json:"id"`
		Name       string  `json:"name"`
		IsProduced bool    `json:"is_produced"`
		IsActive   bool    `json:"is_active"`
		TrackStock bool    `json:"track_stock"`
		MinStock   float64 `json:"min_stock"`
		SalePrice  float64 `json:"sale_price"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("crear: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if created.ID == "" || created.Name != "Pan francés" || !created.IsProduced || !created.IsActive || !created.TrackStock {
		t.Fatalf("crear: respuesta inesperada: %+v", created)
	}
	if created.MinStock != 20 || created.SalePrice != 500 {
		t.Fatalf("crear: decimales inesperados: %+v", created)
	}

	// GET /api/v1/products con filtros
	status, body = do(t, app, http.MethodGet, "/api/v1/products?is_produced=true&search=pan", "", token)
	if status != fiber.StatusOK {
		t.Fatalf("listar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	var products []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &products); err != nil {
		t.Fatalf("listar: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if len(products) != 1 || products[0].ID != created.ID {
		t.Fatalf("listar: products = %+v, se esperaba solo el producto creado", products)
	}

	// GET /api/v1/products/:id
	status, body = do(t, app, http.MethodGet, "/api/v1/products/"+created.ID, "", token)
	if status != fiber.StatusOK {
		t.Fatalf("obtener: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	// PUT /api/v1/products/:id (solo cambia el precio de venta)
	status, body = do(t, app, http.MethodPut, "/api/v1/products/"+created.ID,
		`{"sale_price":550.75}`, token)
	if status != fiber.StatusOK {
		t.Fatalf("actualizar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	var updated struct {
		Name      string  `json:"name"`
		SalePrice float64 `json:"sale_price"`
		IsSold    bool    `json:"is_sold"`
	}
	if err := json.Unmarshal([]byte(body), &updated); err != nil {
		t.Fatalf("actualizar: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if updated.Name != "Pan francés" || updated.SalePrice != 550.75 || !updated.IsSold {
		t.Errorf("actualizar: respuesta inesperada: %+v", updated)
	}

	// DELETE /api/v1/products/:id (soft delete)
	status, body = do(t, app, http.MethodDelete, "/api/v1/products/"+created.ID, "", token)
	if status != fiber.StatusNoContent {
		t.Fatalf("borrar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNoContent, body)
	}

	// El producto ya no existe para la API.
	status, body = do(t, app, http.MethodGet, "/api/v1/products/"+created.ID, "", token)
	if status != fiber.StatusNotFound {
		t.Fatalf("obtener tras borrar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}
}
func TestMeUpdateSinTokenResponde401(t *testing.T) {
	app, _ := newTestApp(t)

	status, body := do(t, app, http.MethodPut, "/api/v1/auth/me", `{"full_name":"María Gómez"}`, "")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
}

func TestMeUpdateCambiaSoloLosCamposEnviados(t *testing.T) {
	app, repo := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	// role e is_active viajan en el body a propósito: el endpoint debe ignorarlos.
	status, body := do(t, app, http.MethodPut, "/api/v1/auth/me",
		`{"full_name":"  María Gómez  ","phone":"1144440000","role":"admin","is_active":false}`, token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var updated struct {
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Phone    string `json:"phone"`
		Role     string `json:"role"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.Unmarshal([]byte(body), &updated); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if updated.FullName != "María Gómez" || updated.Phone != "1144440000" {
		t.Errorf("perfil = %+v, se esperaban los campos actualizados y recortados", updated)
	}
	if updated.Email != "maria@pan001.com" {
		t.Errorf("email = %q, el email no debía cambiar", updated.Email)
	}
	if updated.Role != model.RoleVendedor || !updated.IsActive {
		t.Errorf("role/is_active = %q/%v, el propio usuario no debe poder cambiarlos", updated.Role, updated.IsActive)
	}
	if strings.Contains(body, "password") {
		t.Errorf("la respuesta no debe exponer la contraseña: %s", body)
	}

	stored := repo.byEmail["maria@pan001.com"]
	if stored.FullName != "María Gómez" || stored.Phone != "1144440000" {
		t.Errorf("el cambio no quedó persistido: %+v", stored)
	}
}

func TestMeUpdateNombreVacioResponde400(t *testing.T) {
	app, _ := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	status, body := do(t, app, http.MethodPut, "/api/v1/auth/me", `{"full_name":"   "}`, token)

	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
	}
}

func TestMeUpdateEmailDuplicadoResponde409(t *testing.T) {
	app, _ := newTestApp(t)
	register(t, app, "maria@pan001.com", model.RoleVendedor)
	token := register(t, app, "pedro@pan001.com", model.RoleVendedor)

	status, body := do(t, app, http.MethodPut, "/api/v1/auth/me", `{"email":"maria@pan001.com"}`, token)

	if status != fiber.StatusConflict {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusConflict, body)
	}
}

func TestChangePasswordEndToEnd(t *testing.T) {
	app, _ := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	status, body := do(t, app, http.MethodPut, "/api/v1/auth/me/password",
		`{"current_password":"secreto123","new_password":"nuevaClave123"}`, token)
	if status != fiber.StatusNoContent {
		t.Fatalf("cambiar contraseña: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNoContent, body)
	}
	if body != "" {
		t.Errorf("body = %q, el cambio de contraseña no debe devolver contenido", body)
	}

	// La contraseña nueva funciona y la anterior ya no.
	status, body = do(t, app, http.MethodPost, "/api/v1/auth/login",
		`{"email":"maria@pan001.com","password":"nuevaClave123"}`, "")
	if status != fiber.StatusOK {
		t.Fatalf("login con la contraseña nueva: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	status, body = do(t, app, http.MethodPost, "/api/v1/auth/login",
		`{"email":"maria@pan001.com","password":"secreto123"}`, "")
	if status != fiber.StatusUnauthorized {
		t.Fatalf("login con la contraseña anterior: status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
}

func TestChangePasswordActualIncorrectaResponde400(t *testing.T) {
	app, _ := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	status, body := do(t, app, http.MethodPut, "/api/v1/auth/me/password",
		`{"current_password":"incorrecta","new_password":"nuevaClave123"}`, token)

	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
	}
	if !strings.Contains(body, "la contraseña actual no es correcta") {
		t.Errorf("body = %q, se esperaba el mensaje de contraseña actual incorrecta", body)
	}
}

func TestChangePasswordDemasiadoCortaResponde400(t *testing.T) {
	app, _ := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	status, body := do(t, app, http.MethodPut, "/api/v1/auth/me/password",
		`{"current_password":"secreto123","new_password":"corta"}`, token)

	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
	}
	if !strings.Contains(body, "new_password") {
		t.Errorf("body = %q, se esperaba el nombre del campo en el mensaje", body)
	}
}

func TestChangePasswordSinTokenResponde401(t *testing.T) {
	app, _ := newTestApp(t)

	status, body := do(t, app, http.MethodPut, "/api/v1/auth/me/password",
		`{"current_password":"secreto123","new_password":"nuevaClave123"}`, "")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
}
