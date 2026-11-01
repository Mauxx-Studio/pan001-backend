package handler_test

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
	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
	"github.com/pan001/backend/internal/service"
)

// memContactRepository es un repositorio de terceros en memoria para no
// depender de PostgreSQL.
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

// newContactTestApp monta las rutas de terceros sin middleware JWT: la
// protección del grupo se valida en internal/router/router_test.go.
func newContactTestApp(repo *memContactRepository) *fiber.App {
	contactHandler := handler.NewContactHandler(service.NewContactService(repo))

	app := fiber.New()
	app.Post("/contacts", contactHandler.Create)
	app.Get("/contacts", contactHandler.List)
	app.Get("/contacts/:id", contactHandler.GetByID)
	app.Put("/contacts/:id", contactHandler.Update)
	app.Delete("/contacts/:id", contactHandler.Delete)
	return app
}

// doContact hace la petición y devuelve status + body.
func doContact(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
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

// contactPayload refleja la respuesta pública de un tercero.
type contactPayload struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	BusinessName string    `json:"business_name"`
	TaxID        string    `json:"tax_id"`
	Email        string    `json:"email"`
	Phone        string    `json:"phone"`
	WhatsApp     string    `json:"whatsapp"`
	Address      string    `json:"address"`
	City         string    `json:"city"`
	Notes        string    `json:"notes"`
	IsClient     bool      `json:"is_client"`
	IsSupplier   bool      `json:"is_supplier"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// decodeContact deserializa una respuesta de un tercero.
func decodeContact(t *testing.T, body string) contactPayload {
	t.Helper()

	var contact contactPayload
	if err := json.Unmarshal([]byte(body), &contact); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	return contact
}

func TestCreateContactResponde201(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())

	status, body := doContact(t, app, http.MethodPost, "/contacts", `{
		"name": "Panadería El Sol",
		"business_name": "El Sol SRL",
		"tax_id": "30-12345678-9",
		"email": "contacto@elsol.com",
		"phone": "1122334455",
		"whatsapp": "1122334455",
		"address": "Av. San Martín 123",
		"city": "CABA",
		"notes": "Cliente mayorista",
		"is_client": true,
		"is_supplier": false
	}`)
	if status != fiber.StatusCreated {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}

	contact := decodeContact(t, body)
	if _, err := uuid.Parse(contact.ID); err != nil {
		t.Errorf("id = %q, se esperaba un UUID válido", contact.ID)
	}
	if contact.Name != "Panadería El Sol" || contact.BusinessName != "El Sol SRL" {
		t.Errorf("name/business_name = %q/%q, se esperaban los datos enviados", contact.Name, contact.BusinessName)
	}
	if contact.TaxID != "30-12345678-9" || contact.Email != "contacto@elsol.com" {
		t.Errorf("tax_id/email = %q/%q, se esperaban los datos enviados", contact.TaxID, contact.Email)
	}
	if contact.Phone != "1122334455" || contact.WhatsApp != "1122334455" || contact.City != "CABA" {
		t.Errorf("phone/whatsapp/city = %q/%q/%q, se esperaban los datos enviados", contact.Phone, contact.WhatsApp, contact.City)
	}
	if contact.Address != "Av. San Martín 123" || contact.Notes != "Cliente mayorista" {
		t.Errorf("address/notes = %q/%q, se esperaban los datos enviados", contact.Address, contact.Notes)
	}
	if !contact.IsClient || contact.IsSupplier {
		t.Errorf("is_client/is_supplier = %v/%v, se esperaba true/false", contact.IsClient, contact.IsSupplier)
	}
	if !contact.IsActive {
		t.Error("is_active = false, se esperaba true por defecto")
	}
	if contact.CreatedAt.IsZero() || contact.UpdatedAt.IsZero() {
		t.Error("created_at/updated_at vacíos, se esperaban las fechas de creación")
	}
}

func TestCreateContactSoloRequiereNombre(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())

	status, body := doContact(t, app, http.MethodPost, "/contacts", `{"name":"  Solo Nombre  "}`)
	if status != fiber.StatusCreated {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}

	contact := decodeContact(t, body)
	if contact.Name != "Solo Nombre" {
		t.Errorf("name = %q, se esperaba el nombre sin espacios", contact.Name)
	}
	if contact.IsClient || contact.IsSupplier {
		t.Errorf("is_client/is_supplier = %v/%v, se esperaba false/false por defecto", contact.IsClient, contact.IsSupplier)
	}
	if !contact.IsActive {
		t.Error("is_active = false, se esperaba true por defecto")
	}
}

func TestCreateContactBodyInvalidoResponde400(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())

	casos := map[string]string{
		"sin nombre":       `{}`,
		"nombre vacío":     `{"name":""}`,
		"nombre en blanco": `{"name":"   "}`,
		"email inválido":   `{"name":"Panadería","email":"no-es-email"}`,
		"nombre muy largo": `{"name":"` + strings.Repeat("a", 151) + `"}`,
		"json malformado":  `{"name":`,
	}

	for name, body := range casos {
		t.Run(name, func(t *testing.T) {
			status, respBody := doContact(t, app, http.MethodPost, "/contacts", body)
			if status != fiber.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, respBody)
			}
			// El error handler por defecto de Fiber responde el mensaje en texto.
			if strings.TrimSpace(respBody) == "" {
				t.Error("body vacío, se esperaba un mensaje de validación")
			}
		})
	}
}

// createContact crea un tercero por HTTP y devuelve su representación.
func createContact(t *testing.T, app *fiber.App, body string) contactPayload {
	t.Helper()

	status, respBody := doContact(t, app, http.MethodPost, "/contacts", body)
	if status != fiber.StatusCreated {
		t.Fatalf("no se pudo crear el tercero de prueba: status %d (body: %s)", status, respBody)
	}
	return decodeContact(t, respBody)
}

// listContacts hace GET /contacts con la query indicada y deserializa la lista.
func listContacts(t *testing.T, app *fiber.App, query string) []contactPayload {
	t.Helper()

	status, body := doContact(t, app, http.MethodGet, "/contacts"+query, "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var contacts []contactPayload
	if err := json.Unmarshal([]byte(body), &contacts); err != nil {
		t.Fatalf("respuesta no es un JSON de lista válido: %v (body: %s)", err, body)
	}
	return contacts
}

func TestListContactsConFiltros(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())

	createContact(t, app, `{"name":"Panadería El Sol","business_name":"El Sol SRL","is_client":true}`)
	createContact(t, app, `{"name":"Molino Sur","tax_id":"30-99999999-1","is_supplier":true}`)
	createContact(t, app, `{"name":"Cliente Antiguo","email":"antiguo@elsol.com","is_client":true,"is_active":false}`)

	casos := []struct {
		query string
		want  int
	}{
		{"", 3},
		{"?is_client=true", 2},
		{"?is_client=true&is_active=true", 1},
		{"?is_supplier=true", 1},
		{"?is_client=false", 1},   // solo Molino Sur no es cliente
		{"?is_supplier=false", 2}, // los otros dos no son proveedores
		{"?search=molino", 1},
		{"?search=30-99999999", 1},
		{"?search=%20ANTIGUO@ELSOL.COM%20", 1},
		{"?search=no-existe", 0},
	}

	for _, caso := range casos {
		t.Run(caso.query, func(t *testing.T) {
			contacts := listContacts(t, app, caso.query)
			if len(contacts) != caso.want {
				t.Fatalf("len(contacts) = %d, se esperaban %d (%+v)", len(contacts), caso.want, contacts)
			}
		})
	}

	// Sin coincidencias la respuesta es [], no null.
	status, body := doContact(t, app, http.MethodGet, "/contacts?search=no-existe", "")
	if status != fiber.StatusOK || strings.TrimSpace(body) != "[]" {
		t.Errorf("status/body = %d/%q, se esperaba 200 con []", status, body)
	}
}

func TestListContactsQueryInvalidaResponde400(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())

	casos := map[string]string{
		"is_client":   "?is_client=si",
		"is_supplier": "?is_supplier=1x",
		"is_active":   "?is_active=no",
	}

	for param, query := range casos {
		t.Run(param, func(t *testing.T) {
			status, body := doContact(t, app, http.MethodGet, "/contacts"+query, "")
			if status != fiber.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
			}
			if !strings.Contains(body, param) {
				t.Errorf("body = %q, se esperaba mencionar el parámetro %q", body, param)
			}
		})
	}
}

func TestGetContactPorID(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())
	created := createContact(t, app, `{"name":"Panadería El Sol","city":"CABA"}`)

	status, body := doContact(t, app, http.MethodGet, "/contacts/"+created.ID, "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	contact := decodeContact(t, body)
	if contact.ID != created.ID || contact.Name != "Panadería El Sol" || contact.City != "CABA" {
		t.Errorf("respuesta inesperada: %+v", contact)
	}

	// UUID inexistente → 404 con el mensaje de dominio.
	status, body = doContact(t, app, http.MethodGet, "/contacts/"+uuid.New().String(), "")
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}
	if !strings.Contains(body, "tercero no encontrado") {
		t.Errorf("body = %q, se esperaba el mensaje de tercero no encontrado", body)
	}

	// ID con formato inválido → 400.
	status, body = doContact(t, app, http.MethodGet, "/contacts/no-es-un-uuid", "")
	if status != fiber.StatusBadRequest {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
	}
}

func TestUpdateContactParcial(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())
	created := createContact(t, app,
		`{"name":"Panadería El Sol","phone":"1122334455","city":"CABA","is_client":true}`)

	status, body := doContact(t, app, http.MethodPut, "/contacts/"+created.ID,
		`{"phone":" 1199998888 ","email":"VENTAS@ElSol.com","is_supplier":true}`)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	updated := decodeContact(t, body)
	if updated.ID != created.ID {
		t.Errorf("id = %q, se esperaba %q", updated.ID, created.ID)
	}
	if updated.Phone != "1199998888" {
		t.Errorf("phone = %q, se esperaba el teléfono nuevo sin espacios", updated.Phone)
	}
	if updated.Email != "ventas@elsol.com" {
		t.Errorf("email = %q, se esperaba el email normalizado", updated.Email)
	}
	if !updated.IsSupplier {
		t.Error("is_supplier = false, se esperaba true")
	}
	// Los campos ausentes no se tocan.
	if updated.Name != "Panadería El Sol" || updated.City != "CABA" || !updated.IsClient || !updated.IsActive {
		t.Errorf("campos no enviados modificados: %+v", updated)
	}

	// El cambio quedó persistido.
	_, body = doContact(t, app, http.MethodGet, "/contacts/"+created.ID, "")
	reloaded := decodeContact(t, body)
	if reloaded.Phone != "1199998888" || !reloaded.IsSupplier {
		t.Errorf("el tercero recargado no conserva los cambios: %+v", reloaded)
	}
}

func TestUpdateContactBodyInvalidoResponde400(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())
	created := createContact(t, app, `{"name":"Panadería El Sol"}`)

	casos := map[string]string{
		"nombre vacío":     `{"name":""}`,
		"nombre en blanco": `{"name":"   "}`,
		"email inválido":   `{"email":"no-es-email"}`,
		"json malformado":  `{"name":`,
	}

	for name, body := range casos {
		t.Run(name, func(t *testing.T) {
			status, respBody := doContact(t, app, http.MethodPut, "/contacts/"+created.ID, body)
			if status != fiber.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, respBody)
			}
		})
	}

	// El nombre original sigue intacto.
	_, body := doContact(t, app, http.MethodGet, "/contacts/"+created.ID, "")
	if contact := decodeContact(t, body); contact.Name != "Panadería El Sol" {
		t.Errorf("name = %q, el nombre no debería haber cambiado", contact.Name)
	}
}

func TestUpdateContactNoEncontradoResponde404(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())

	status, body := doContact(t, app, http.MethodPut, "/contacts/"+uuid.New().String(), `{"name":"Nuevo"}`)
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}
}

func TestDeleteContactResponde204(t *testing.T) {
	app := newContactTestApp(newMemContactRepository())
	created := createContact(t, app, `{"name":"Panadería El Sol"}`)

	status, body := doContact(t, app, http.MethodDelete, "/contacts/"+created.ID, "")
	if status != fiber.StatusNoContent {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusNoContent, body)
	}
	if strings.TrimSpace(body) != "" {
		t.Errorf("body = %q, se esperaba vacío en un 204", body)
	}

	// Soft delete: el tercero ya no se puede obtener.
	status, _ = doContact(t, app, http.MethodGet, "/contacts/"+created.ID, "")
	if status != fiber.StatusNotFound {
		t.Fatalf("status = %d, se esperaba %d después del delete", status, fiber.StatusNotFound)
	}

	// Borrar dos veces responde 404.
	status, body = doContact(t, app, http.MethodDelete, "/contacts/"+created.ID, "")
	if status != fiber.StatusNotFound {
		t.Fatalf("segundo delete: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}

	// ID inválido → 400.
	status, _ = doContact(t, app, http.MethodDelete, "/contacts/no-es-un-uuid", "")
	if status != fiber.StatusBadRequest {
		t.Fatalf("id inválido: status = %d, se esperaba %d", status, fiber.StatusBadRequest)
	}
}
