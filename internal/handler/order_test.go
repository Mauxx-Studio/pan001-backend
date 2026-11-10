package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/handler"
	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
	"github.com/pan001/backend/internal/service"
)

// memOrderRepository es un repositorio de pedidos en memoria para no depender
// de PostgreSQL. Guarda copias, así el service no ve mutaciones de su propio
// objeto antes de confirmar los cambios.
type memOrderRepository struct {
	byID  map[uuid.UUID]*model.Order
	items map[uuid.UUID][]*model.OrderItem
}

func newMemOrderRepository() *memOrderRepository {
	return &memOrderRepository{
		byID:  map[uuid.UUID]*model.Order{},
		items: map[uuid.UUID][]*model.OrderItem{},
	}
}

func (r *memOrderRepository) Create(_ context.Context, order *model.Order) error {
	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}
	now := time.Now().UTC()
	order.CreatedAt, order.UpdatedAt = now, now

	stored := make([]*model.OrderItem, 0, len(order.Items))
	for i := range order.Items {
		if order.Items[i].ID == uuid.Nil {
			order.Items[i].ID = uuid.New()
		}
		order.Items[i].OrderID = order.ID
		order.Items[i].CreatedAt, order.Items[i].UpdatedAt = now, now
		stored = append(stored, cloneMemOrderItem(&order.Items[i], order.ID))
	}
	r.byID[order.ID] = cloneMemOrder(order)
	r.items[order.ID] = stored
	return nil
}

func (r *memOrderRepository) FindByID(_ context.Context, id uuid.UUID) (*model.Order, error) {
	order, ok := r.byID[id]
	if !ok || order.DeletedAt.Valid {
		return nil, model.ErrOrderNotFound
	}

	found := cloneMemOrder(order)
	for _, item := range r.items[id] {
		if item.DeletedAt.Valid {
			continue
		}
		found.Items = append(found.Items, *cloneMemOrderItem(item, id))
	}
	return found, nil
}

func (r *memOrderRepository) List(_ context.Context, filters repository.OrderFilters) ([]model.Order, error) {
	orders := make([]model.Order, 0, len(r.byID))
	for _, order := range r.byID {
		if order.DeletedAt.Valid {
			continue
		}
		if filters.ContactID != nil && order.ContactID != *filters.ContactID {
			continue
		}
		if filters.Status != "" && order.Status != filters.Status {
			continue
		}
		if filters.PaymentStatus != "" && order.PaymentStatus != filters.PaymentStatus {
			continue
		}
		if filters.From != nil && order.OrderedAt.Before(*filters.From) {
			continue
		}
		if filters.To != nil && order.OrderedAt.After(*filters.To) {
			continue
		}
		if search := strings.ToLower(strings.TrimSpace(filters.Search)); search != "" &&
			!strings.Contains(strings.ToLower(order.Code+" "+order.Notes), search) {
			continue
		}
		orders = append(orders, *cloneMemOrder(order))
	}

	sort.Slice(orders, func(i, j int) bool { return orders[i].OrderedAt.After(orders[j].OrderedAt) })
	return orders, nil
}

func (r *memOrderRepository) Update(_ context.Context, order *model.Order) error {
	current, ok := r.byID[order.ID]
	if !ok || current.DeletedAt.Valid {
		return model.ErrOrderNotFound
	}
	updated := cloneMemOrder(order)
	updated.CreatedAt = current.CreatedAt
	updated.UpdatedAt = time.Now().UTC()
	r.byID[order.ID] = updated

	for _, stored := range r.items[order.ID] {
		for i := range order.Items {
			if stored.ID != order.Items[i].ID {
				continue
			}
			stored.ProductID = order.Items[i].ProductID
			stored.ProductName = order.Items[i].ProductName
			stored.Unit = order.Items[i].Unit
			stored.Quantity = order.Items[i].Quantity
			stored.UnitPrice = order.Items[i].UnitPrice
			stored.LineTotal = order.Items[i].LineTotal
			stored.Notes = order.Items[i].Notes
			stored.UpdatedAt = updated.UpdatedAt
		}
	}
	return nil
}

func (r *memOrderRepository) Delete(_ context.Context, id uuid.UUID) error {
	order, ok := r.byID[id]
	if !ok || order.DeletedAt.Valid {
		return model.ErrOrderNotFound
	}
	now := time.Now().UTC()
	order.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}
	for _, item := range r.items[id] {
		item.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}
	}
	return nil
}

func (r *memOrderRepository) NextDailySequence(_ context.Context, day time.Time) (int64, error) {
	prefix := model.OrderCodePrefix(day)

	var last int64
	for _, order := range r.byID {
		if !strings.HasPrefix(order.Code, prefix) {
			continue
		}
		sequence, err := strconv.ParseInt(strings.TrimPrefix(order.Code, prefix), 10, 64)
		if err != nil {
			continue
		}
		if sequence > last {
			last = sequence
		}
	}
	return last + 1, nil
}

func (r *memOrderRepository) CreateItems(_ context.Context, items []model.OrderItem) error {
	now := time.Now().UTC()
	for i := range items {
		if items[i].ID == uuid.Nil {
			items[i].ID = uuid.New()
		}
		items[i].CreatedAt, items[i].UpdatedAt = now, now
		r.items[items[i].OrderID] = append(r.items[items[i].OrderID], cloneMemOrderItem(&items[i], items[i].OrderID))
	}
	return nil
}

func (r *memOrderRepository) UpdateItem(_ context.Context, item *model.OrderItem) error {
	for _, stored := range r.items[item.OrderID] {
		if stored.ID != item.ID {
			continue
		}
		stored.ProductID = item.ProductID
		stored.ProductName = item.ProductName
		stored.Unit = item.Unit
		stored.Quantity = item.Quantity
		stored.QuantityDelivered = item.QuantityDelivered
		stored.UnitPrice = item.UnitPrice
		stored.LineTotal = item.LineTotal
		stored.Notes = item.Notes
		stored.UpdatedAt = time.Now().UTC()
		return nil
	}
	return model.ErrOrderItemNotFound
}

func (r *memOrderRepository) SoftDeleteItems(_ context.Context, orderID uuid.UUID, ids []uuid.UUID) error {
	now := time.Now().UTC()
	for _, item := range r.items[orderID] {
		for _, id := range ids {
			if item.ID == id {
				item.DeletedAt = gorm.DeletedAt{Time: now, Valid: true}
			}
		}
	}
	return nil
}

func (r *memOrderRepository) WithTransaction(ctx context.Context, fn func(tx repository.OrderRepository) error) error {
	return fn(r)
}

// memOrderRepository implementa repository.OrderRepository.
var _ repository.OrderRepository = (*memOrderRepository)(nil)

// cloneMemOrder copia la cabecera sin compartir las líneas.
func cloneMemOrder(order *model.Order) *model.Order {
	copied := *order
	if order.DeliveryAt != nil {
		deliveryAt := *order.DeliveryAt
		copied.DeliveryAt = &deliveryAt
	}
	if order.ParentOrderID != nil {
		parentID := *order.ParentOrderID
		copied.ParentOrderID = &parentID
	}
	if order.CreatedBy != nil {
		createdBy := *order.CreatedBy
		copied.CreatedBy = &createdBy
	}
	copied.Items = nil
	return &copied
}

// cloneMemOrderItem copia una línea asignándole su pedido.
func cloneMemOrderItem(item *model.OrderItem, orderID uuid.UUID) *model.OrderItem {
	copied := *item
	copied.OrderID = orderID
	return &copied
}

// orderAPIFixture monta la API de pedidos con un tercero y un producto ya
// cargados. Las rutas se registran sin middleware JWT: la protección del grupo
// se valida en internal/router/router_test.go.
type orderAPIFixture struct {
	app      *fiber.App
	orders   *memOrderRepository
	contacts *memContactRepository
	products *memProductRepository
	contact  *model.Contact
	product  *model.Product
}

func newOrderAPIFixture(t *testing.T) *orderAPIFixture {
	t.Helper()

	orderRepo := newMemOrderRepository()
	contactRepo := newMemContactRepository()
	productRepo := newMemProductRepository()

	contact := &model.Contact{Name: "Panadería Central", IsClient: true, IsActive: true}
	if err := contactRepo.Create(context.Background(), contact); err != nil {
		t.Fatalf("creando tercero de prueba: %v", err)
	}
	product := addProductToRepo(t, productRepo, "Pan francés", "unidad", "150.50")

	orderHandler := handler.NewOrderHandler(service.NewOrderService(orderRepo, contactRepo, productRepo))
	app := fiber.New()
	app.Post("/orders", orderHandler.Create)
	app.Get("/orders", orderHandler.List)
	app.Get("/orders/:id", orderHandler.GetByID)
	app.Put("/orders/:id", orderHandler.Update)
	app.Delete("/orders/:id", orderHandler.Delete)

	return &orderAPIFixture{
		app:      app,
		orders:   orderRepo,
		contacts: contactRepo,
		products: productRepo,
		contact:  contact,
		product:  product,
	}
}

// addProductToRepo carga un producto en el catálogo en memoria.
func addProductToRepo(t *testing.T, repo *memProductRepository, name, unit, salePrice string) *model.Product {
	t.Helper()

	product := &model.Product{
		Name:      name,
		Unit:      unit,
		SalePrice: decimal.RequireFromString(salePrice),
		IsSold:    true,
		IsActive:  true,
	}
	if err := repo.Create(context.Background(), product); err != nil {
		t.Fatalf("creando producto %q: %v", name, err)
	}
	return product
}

// orderItemPayload refleja la respuesta pública de una línea.
type orderItemPayload struct {
	ID                string  `json:"id"`
	ProductID         string  `json:"product_id"`
	ProductName       string  `json:"product_name"`
	Unit              string  `json:"unit"`
	Quantity          float64 `json:"quantity"`
	QuantityDelivered float64 `json:"quantity_delivered"`
	UnitPrice         float64 `json:"unit_price"`
	LineTotal         float64 `json:"line_total"`
	Notes             string  `json:"notes"`
}

// orderPayload refleja la respuesta pública de un pedido. Los decimales se
// deserializan como float64 para comprobar que la API los devuelve como números
// JSON y no como cadenas.
type orderPayload struct {
	ID             string             `json:"id"`
	Code           string             `json:"code"`
	ContactID      string             `json:"contact_id"`
	Status         string             `json:"status"`
	OrderedAt      time.Time          `json:"ordered_at"`
	DeliveryAt     *time.Time         `json:"delivery_at"`
	DeliveryType   string             `json:"delivery_type"`
	Subtotal       float64            `json:"subtotal"`
	DiscountAmount float64            `json:"discount_amount"`
	FinalAmount    float64            `json:"final_amount"`
	PaymentMethod  string             `json:"payment_method"`
	PaymentStatus  string             `json:"payment_status"`
	AmountPaid     float64            `json:"amount_paid"`
	ParentOrderID  string             `json:"parent_order_id"`
	Notes          string             `json:"notes"`
	CreatedBy      string             `json:"created_by"`
	IsActive       bool               `json:"is_active"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
	Items          []orderItemPayload `json:"items"`
}

// doOrder ejecuta una petición HTTP contra la app de prueba de pedidos.
func doOrder(t *testing.T, app *fiber.App, method, path, body string) (int, string) {
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

// decodeOrder deserializa la respuesta de un pedido.
func decodeOrder(t *testing.T, body string) orderPayload {
	t.Helper()

	var order orderPayload
	if err := json.Unmarshal([]byte(body), &order); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	return order
}

// createOrderHTTP crea un pedido por HTTP y devuelve su representación.
func createOrderHTTP(t *testing.T, app *fiber.App, payload map[string]any) orderPayload {
	t.Helper()

	status, body := doOrder(t, app, http.MethodPost, "/orders", jsonBody(t, payload))
	if status != fiber.StatusCreated {
		t.Fatalf("crear: status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}
	return decodeOrder(t, body)
}

// listOrdersHTTP lista pedidos por HTTP aplicando la query string indicada.
func listOrdersHTTP(t *testing.T, app *fiber.App, query string) []orderPayload {
	t.Helper()

	status, body := doOrder(t, app, http.MethodGet, "/orders"+query, "")
	if status != fiber.StatusOK {
		t.Fatalf("listar: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var orders []orderPayload
	if err := json.Unmarshal([]byte(body), &orders); err != nil {
		t.Fatalf("listar: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	return orders
}

// assertAmount compara dos importes de la respuesta con la tolerancia propia de
// un float64.
func assertAmount(t *testing.T, field string, got, want float64) {
	t.Helper()

	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("%s = %v, se esperaba %v", field, got, want)
	}
}

func TestCreateOrderResponde201(t *testing.T) {
	f := newOrderAPIFixture(t)

	order := createOrderHTTP(t, f.app, map[string]any{
		"contact_id":      f.contact.ID.String(),
		"delivery_type":   "delivery",
		"payment_method":  "cash",
		"discount_amount": "50.50",
		"amount_paid":     100,
		"notes":           "  entregar temprano  ",
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 2.5, "notes": "  cortado fino  "},
		},
	})

	if _, err := uuid.Parse(order.ID); err != nil {
		t.Errorf("id = %q, se esperaba un UUID (%v)", order.ID, err)
	}
	if want := model.NewOrderCode(order.OrderedAt, 1); order.Code != want {
		t.Errorf("code = %q, se esperaba %q", order.Code, want)
	}
	if order.ContactID != f.contact.ID.String() {
		t.Errorf("contact_id = %q, se esperaba %q", order.ContactID, f.contact.ID)
	}
	if order.Status != model.OrderStatusConfirmed {
		t.Errorf("status = %q, se esperaba %q", order.Status, model.OrderStatusConfirmed)
	}
	if order.PaymentStatus != model.PaymentStatusPending {
		t.Errorf("payment_status = %q, se esperaba %q", order.PaymentStatus, model.PaymentStatusPending)
	}
	if order.DeliveryType != model.DeliveryTypeDelivery || order.PaymentMethod != model.PaymentMethodCash {
		t.Errorf("delivery_type/payment_method = %q/%q", order.DeliveryType, order.PaymentMethod)
	}
	if order.Notes != "entregar temprano" {
		t.Errorf("notes = %q, se esperaba recortado", order.Notes)
	}
	if order.CreatedBy != "" {
		t.Errorf("created_by = %q, se esperaba vacío sin usuario autenticado", order.CreatedBy)
	}
	if !order.IsActive {
		t.Error("is_active = false, se esperaba true")
	}
	if order.DeliveryAt != nil {
		t.Errorf("delivery_at = %v, se esperaba null", order.DeliveryAt)
	}

	// 2.5 * 150.50 = 376.25; con 50.50 de descuento quedan 325.75.
	assertAmount(t, "subtotal", order.Subtotal, 376.25)
	assertAmount(t, "discount_amount", order.DiscountAmount, 50.5)
	assertAmount(t, "final_amount", order.FinalAmount, 325.75)
	assertAmount(t, "amount_paid", order.AmountPaid, 100)

	if len(order.Items) != 1 {
		t.Fatalf("len(items) = %d, se esperaba 1", len(order.Items))
	}
	line := order.Items[0]
	if _, err := uuid.Parse(line.ID); err != nil {
		t.Errorf("id de la línea = %q, se esperaba un UUID (%v)", line.ID, err)
	}
	if line.ProductID != f.product.ID.String() {
		t.Errorf("product_id = %q, se esperaba %q", line.ProductID, f.product.ID)
	}
	if line.ProductName != "Pan francés" || line.Unit != "unidad" {
		t.Errorf("snapshot = %q/%q, se esperaban Pan francés/unidad", line.ProductName, line.Unit)
	}
	assertAmount(t, "quantity", line.Quantity, 2.5)
	assertAmount(t, "quantity_delivered", line.QuantityDelivered, 0)
	assertAmount(t, "unit_price", line.UnitPrice, 150.5)
	assertAmount(t, "line_total", line.LineTotal, 376.25)
	if line.Notes != "cortado fino" {
		t.Errorf("notes de la línea = %q, se esperaba recortado", line.Notes)
	}

	// El precio no se recibe del cliente: si viene en el body, se ignora.
	otro := createOrderHTTP(t, f.app, map[string]any{
		"contact_id": f.contact.ID.String(),
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 1, "unit_price": 1},
		},
	})
	if len(otro.Items) != 1 {
		t.Fatalf("len(items) = %d, se esperaba 1", len(otro.Items))
	}
	assertAmount(t, "unit_price", otro.Items[0].UnitPrice, 150.5)
	assertAmount(t, "line_total", otro.Items[0].LineTotal, 150.5)
	assertAmount(t, "final_amount", otro.FinalAmount, 150.5)
	// Los códigos son correlativos por día.
	if otro.Code == order.Code {
		t.Errorf("los dos pedidos comparten el código %q", otro.Code)
	}
}

func TestCreateOrderValidaBody(t *testing.T) {
	f := newOrderAPIFixture(t)
	item := []map[string]any{{"product_id": f.product.ID.String(), "quantity": 1}}

	casos := []struct {
		name string
		body string
	}{
		{name: "body vacío", body: "{}"},
		{name: "json inválido", body: `{"contact_id":`},
		{name: "contact_id no es uuid", body: `{"contact_id":"no-es-uuid","items":[]}`},
		{name: "contact inexistente", body: jsonBody(t, map[string]any{"contact_id": uuid.New().String()})},
		{name: "status inválido", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(), "status": "en_camino", "items": item,
		})},
		{name: "delivery_type inválido", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(), "delivery_type": "drone", "items": item,
		})},
		{name: "payment_method inválido", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(), "payment_method": "bitcoin", "items": item,
		})},
		{name: "payment_status inválido", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(), "payment_status": "cobrado", "items": item,
		})},
		{name: "delivery_at inválido", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(), "delivery_at": "mañana", "items": item,
		})},
		{name: "product_id inválido", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(),
			"items":      []map[string]any{{"product_id": "pan", "quantity": 1}},
		})},
		{name: "producto inexistente", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(),
			"items":      []map[string]any{{"product_id": uuid.New().String(), "quantity": 1}},
		})},
		{name: "cantidad cero", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(),
			"items":      []map[string]any{{"product_id": f.product.ID.String(), "quantity": 0}},
		})},
		{name: "sin líneas y sin borrador", body: jsonBody(t, map[string]any{
			"contact_id": f.contact.ID.String(), "items": []map[string]any{},
		})},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			status, body := doOrder(t, f.app, http.MethodPost, "/orders", caso.body)
			if status != fiber.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
			}
			errorMessage(t, body)
		})
	}

	// Un borrador sí puede quedar sin líneas.
	draft := createOrderHTTP(t, f.app, map[string]any{
		"contact_id": f.contact.ID.String(),
		"status":     model.OrderStatusDraft,
	})
	if draft.Status != model.OrderStatusDraft || len(draft.Items) != 0 {
		t.Errorf("borrador = %+v, se esperaba draft sin líneas", draft)
	}
	if len(f.orders.byID) != 1 {
		t.Errorf("se guardaron %d pedidos, se esperaba solo el borrador", len(f.orders.byID))
	}
}

func TestListOrdersAplicaFiltros(t *testing.T) {
	f := newOrderAPIFixture(t)

	otroContacto := &model.Contact{Name: "Almacén Sur", IsClient: true, IsActive: true}
	if err := f.contacts.Create(context.Background(), otroContacto); err != nil {
		t.Fatalf("creando segundo tercero: %v", err)
	}

	primero := createOrderHTTP(t, f.app, map[string]any{
		"contact_id": f.contact.ID.String(),
		"notes":      "reparto del martes",
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 1},
		},
	})
	segundo := createOrderHTTP(t, f.app, map[string]any{
		"contact_id":     otroContacto.ID.String(),
		"status":         model.OrderStatusDraft,
		"payment_method": model.PaymentMethodTransfer,
	})
	eliminado := createOrderHTTP(t, f.app, map[string]any{
		"contact_id": f.contact.ID.String(),
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 1},
		},
	})
	if status, body := doOrder(t, f.app, http.MethodDelete, "/orders/"+eliminado.ID, ""); status != fiber.StatusNoContent {
		t.Fatalf("delete: status = %d (body: %s)", status, body)
	}

	casos := []struct {
		name    string
		query   string
		wantLen int
		wantIDs []string
	}{
		{name: "sin filtros", query: "", wantLen: 2},
		{name: "por tercero", query: "?contact_id=" + otroContacto.ID.String(), wantLen: 1, wantIDs: []string{segundo.ID}},
		{name: "por estado", query: "?status=" + model.OrderStatusDraft, wantLen: 1, wantIDs: []string{segundo.ID}},
		{name: "por estado de pago", query: "?payment_status=" + model.PaymentStatusPending, wantLen: 2},
		{name: "por búsqueda en notas", query: "?search=martes", wantLen: 1, wantIDs: []string{primero.ID}},
		{name: "por búsqueda en código", query: "?search=" + primero.Code, wantLen: 1, wantIDs: []string{primero.ID}},
		{name: "por búsqueda sin coincidencias", query: "?search=inexistente", wantLen: 0},
		{name: "desde hoy", query: "?from=" + time.Now().UTC().Format("2006-01-02"), wantLen: 2},
		{name: "hasta ayer", query: "?to=" + time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"), wantLen: 0},
		{name: "hasta hoy incluye el día completo", query: "?to=" + time.Now().UTC().Format("2006-01-02"), wantLen: 2},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			orders := listOrdersHTTP(t, f.app, caso.query)
			if len(orders) != caso.wantLen {
				t.Fatalf("len = %d, se esperaban %d (%+v)", len(orders), caso.wantLen, orders)
			}
			for i, id := range caso.wantIDs {
				if orders[i].ID != id {
					t.Errorf("orders[%d].id = %q, se esperaba %q", i, orders[i].ID, id)
				}
			}
			// El listado no incluye líneas.
			if len(orders) > 0 && orders[0].Items != nil {
				t.Errorf("el listado devolvió líneas: %+v", orders[0].Items)
			}
		})
	}

	invalidos := []string{
		"?contact_id=no-es-uuid",
		"?from=ayer",
		"?to=31/12/2025",
		"?status=en_camino",
		"?payment_status=cobrado",
	}
	for _, query := range invalidos {
		t.Run("inválido "+query, func(t *testing.T) {
			status, body := doOrder(t, f.app, http.MethodGet, "/orders"+query, "")
			if status != fiber.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, body)
			}
			errorMessage(t, body)
		})
	}
}

func TestGetOrderRespondeDetalle(t *testing.T) {
	f := newOrderAPIFixture(t)

	created := createOrderHTTP(t, f.app, map[string]any{
		"contact_id":  f.contact.ID.String(),
		"delivery_at": "2025-03-01",
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 2},
		},
	})

	status, body := doOrder(t, f.app, http.MethodGet, "/orders/"+created.ID, "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	order := decodeOrder(t, body)
	if order.ID != created.ID || order.Code != created.Code {
		t.Errorf("detalle = %+v, se esperaba el pedido %s", order, created.ID)
	}
	if len(order.Items) != 1 {
		t.Fatalf("len(items) = %d, se esperaba 1", len(order.Items))
	}
	assertAmount(t, "subtotal", order.Subtotal, 301)
	assertAmount(t, "final_amount", order.FinalAmount, 301)

	// delivery_at de un día llega como medianoche UTC.
	want, err := time.Parse(time.RFC3339, "2025-03-01T00:00:00Z")
	if err != nil {
		t.Fatalf("fecha de prueba inválida: %v", err)
	}
	if order.DeliveryAt == nil || !order.DeliveryAt.Equal(want) {
		t.Errorf("delivery_at = %v, se esperaba %v", order.DeliveryAt, want)
	}

	casos := []struct {
		name   string
		id     string
		status int
	}{
		{name: "inexistente", id: uuid.New().String(), status: fiber.StatusNotFound},
		{name: "uuid inválido", id: "no-es-uuid", status: fiber.StatusBadRequest},
	}
	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			status, body := doOrder(t, f.app, http.MethodGet, "/orders/"+caso.id, "")
			if status != caso.status {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, caso.status, body)
			}
			errorMessage(t, body)
		})
	}
}

func TestUpdateOrderReemplazaLineas(t *testing.T) {
	f := newOrderAPIFixture(t)
	factura := addProductToRepo(t, f.products, "Factura", "unidad", "100")

	created := createOrderHTTP(t, f.app, map[string]any{
		"contact_id": f.contact.ID.String(),
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 1},
		},
	})

	status, body := doOrder(t, f.app, http.MethodPut, "/orders/"+created.ID, jsonBody(t, map[string]any{
		"notes":           "actualizado",
		"discount_amount": "10.00",
		"items": []map[string]any{
			{"id": created.Items[0].ID, "quantity": 2},
			{"product_id": factura.ID.String(), "quantity": 1, "notes": "nueva"},
		},
	}))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	updated := decodeOrder(t, body)
	if updated.Notes != "actualizado" {
		t.Errorf("notes = %q, se esperaba actualizado", updated.Notes)
	}
	// 2 * 150.50 + 100 = 401; con 10 de descuento quedan 391.
	assertAmount(t, "subtotal", updated.Subtotal, 401)
	assertAmount(t, "discount_amount", updated.DiscountAmount, 10)
	assertAmount(t, "final_amount", updated.FinalAmount, 391)
	if len(updated.Items) != 2 {
		t.Fatalf("len(items) = %d, se esperaban 2 (%+v)", len(updated.Items), updated.Items)
	}

	modificada := findOrderItem(t, updated, created.Items[0].ID)
	assertAmount(t, "quantity", modificada.Quantity, 2)
	assertAmount(t, "line_total", modificada.LineTotal, 301)
	if modificada.ProductName != "Pan francés" {
		t.Errorf("product_name = %q, se esperaba conservar el snapshot", modificada.ProductName)
	}

	agregada := findOrderItemByProduct(t, updated, factura.ID.String())
	if agregada.ProductName != "Factura" || agregada.Unit != "unidad" {
		t.Errorf("línea nueva = %+v, se esperaba la factura", agregada)
	}
	assertAmount(t, "unit_price", agregada.UnitPrice, 100)
	if agregada.Notes != "nueva" {
		t.Errorf("notes de la línea nueva = %q", agregada.Notes)
	}

	// El pedido guardado refleja el reemplazo.
	if got := len(f.orders.items[createdID(t, f, created.Code)]); got != 2 {
		t.Errorf("el repositorio guardó %d líneas, se esperaban 2", got)
	}

	// Un PUT sin items deja las líneas como están.
	status, body = doOrder(t, f.app, http.MethodPut, "/orders/"+created.ID, jsonBody(t, map[string]any{
		"payment_status": model.PaymentStatusPaid,
		"amount_paid":    "391.00",
	}))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	sinCambios := decodeOrder(t, body)
	if sinCambios.PaymentStatus != model.PaymentStatusPaid {
		t.Errorf("payment_status = %q, se esperaba paid", sinCambios.PaymentStatus)
	}
	assertAmount(t, "amount_paid", sinCambios.AmountPaid, 391)
	if len(sinCambios.Items) != 2 {
		t.Errorf("len(items) = %d, se esperaban 2 sin tocar", len(sinCambios.Items))
	}
	assertAmount(t, "subtotal", sinCambios.Subtotal, 401)
}

// findOrderItem busca una línea por id.
func findOrderItem(t *testing.T, order orderPayload, id string) orderItemPayload {
	t.Helper()

	for _, item := range order.Items {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("no se encontró la línea %q en %+v", id, order.Items)
	return orderItemPayload{}
}

// findOrderItemByProduct busca la línea de un producto.
func findOrderItemByProduct(t *testing.T, order orderPayload, productID string) orderItemPayload {
	t.Helper()

	for _, item := range order.Items {
		if item.ProductID == productID {
			return item
		}
	}
	t.Fatalf("no se encontró la línea del producto %q en %+v", productID, order.Items)
	return orderItemPayload{}
}

// createdID devuelve el UUID guardado del pedido con el código indicado.
func createdID(t *testing.T, f *orderAPIFixture, code string) uuid.UUID {
	t.Helper()

	for id, order := range f.orders.byID {
		if order.Code == code {
			return id
		}
	}
	t.Fatalf("no se encontró el pedido %q en el repositorio", code)
	return uuid.Nil
}

func TestUpdateOrderCabeceraYVacios(t *testing.T) {
	f := newOrderAPIFixture(t)

	created := createOrderHTTP(t, f.app, map[string]any{
		"contact_id":  f.contact.ID.String(),
		"delivery_at": "2025-03-01",
		"notes":       "inicial",
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 1},
		},
	})

	// Un body vacío no cambia nada.
	status, body := doOrder(t, f.app, http.MethodPut, "/orders/"+created.ID, "{}")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	igual := decodeOrder(t, body)
	if igual.Notes != "inicial" || igual.DeliveryAt == nil || len(igual.Items) != 1 {
		t.Errorf("un body vacío modificó el pedido: %+v", igual)
	}

	// Los campos vacíos borran las opcionales.
	status, body = doOrder(t, f.app, http.MethodPut, "/orders/"+created.ID, jsonBody(t, map[string]any{
		"notes":       "",
		"delivery_at": "",
		"is_active":   false,
	}))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	limpio := decodeOrder(t, body)
	if limpio.Notes != "" {
		t.Errorf("notes = %q, se esperaba vacío", limpio.Notes)
	}
	if limpio.DeliveryAt != nil {
		t.Errorf("delivery_at = %v, se esperaba null", limpio.DeliveryAt)
	}
	if limpio.IsActive {
		t.Error("is_active = true, se esperaba false")
	}
	assertAmount(t, "final_amount", limpio.FinalAmount, 150.5)

	// Un borrador puede quedarse sin líneas.
	if _, body := doOrder(t, f.app, http.MethodPut, "/orders/"+created.ID, jsonBody(t, map[string]any{
		"status": model.OrderStatusDraft,
	})); body == "" {
		t.Fatal("respuesta vacía al pasar a borrador")
	}
	status, body = doOrder(t, f.app, http.MethodPut, "/orders/"+created.ID, jsonBody(t, map[string]any{
		"items": []map[string]any{},
	}))
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	vacio := decodeOrder(t, body)
	if len(vacio.Items) != 0 {
		t.Errorf("len(items) = %d, se esperaban 0", len(vacio.Items))
	}
	assertAmount(t, "subtotal", vacio.Subtotal, 0)
	assertAmount(t, "final_amount", vacio.FinalAmount, 0)
	// Las líneas se borran lógicamente, igual que en DELETE.
	lineas := f.orders.items[createdID(t, f, created.Code)]
	if len(lineas) != 1 {
		t.Fatalf("el repositorio guardó %d líneas, se esperaba 1 (la original)", len(lineas))
	}
	if !lineas[0].DeletedAt.Valid {
		t.Errorf("la línea %s no quedó eliminada", lineas[0].ID)
	}
}

func TestUpdateOrderValida(t *testing.T) {
	f := newOrderAPIFixture(t)
	ajena := uuid.New().String()

	created := createOrderHTTP(t, f.app, map[string]any{
		"contact_id": f.contact.ID.String(),
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 1},
		},
	})

	casos := []struct {
		name   string
		id     string
		body   string
		status int
	}{
		{name: "pedido inexistente", id: uuid.New().String(), body: "{}", status: fiber.StatusNotFound},
		{name: "id inválido", id: "no-es-uuid", body: "{}", status: fiber.StatusBadRequest},
		{name: "json inválido", id: created.ID, body: `{"notes":`, status: fiber.StatusBadRequest},
		{name: "status inválido", id: created.ID, body: jsonBody(t, map[string]any{
			"status": "en_camino",
		}), status: fiber.StatusBadRequest},
		{name: "delivery_type inválido", id: created.ID, body: jsonBody(t, map[string]any{
			"delivery_type": "drone",
		}), status: fiber.StatusBadRequest},
		{name: "payment_status inválido", id: created.ID, body: jsonBody(t, map[string]any{
			"payment_status": "cobrado",
		}), status: fiber.StatusBadRequest},
		{name: "delivery_at inválido", id: created.ID, body: jsonBody(t, map[string]any{
			"delivery_at": "mañana",
		}), status: fiber.StatusBadRequest},
		{name: "contact_id inválido", id: created.ID, body: jsonBody(t, map[string]any{
			"contact_id": "no-es-uuid",
		}), status: fiber.StatusBadRequest},
		{name: "contact inexistente", id: created.ID, body: jsonBody(t, map[string]any{
			"contact_id": ajena,
		}), status: fiber.StatusBadRequest},
		{name: "línea ajena", id: created.ID, body: jsonBody(t, map[string]any{
			"items": []map[string]any{{"id": ajena, "quantity": 1}},
		}), status: fiber.StatusBadRequest},
		{name: "línea nueva sin producto", id: created.ID, body: jsonBody(t, map[string]any{
			"items": []map[string]any{{"quantity": 1}},
		}), status: fiber.StatusBadRequest},
		{name: "línea nueva con cantidad cero", id: created.ID, body: jsonBody(t, map[string]any{
			"items": []map[string]any{{"product_id": f.product.ID.String(), "quantity": 0}},
		}), status: fiber.StatusBadRequest},
		{name: "líneas vacías en pedido confirmado", id: created.ID, body: jsonBody(t, map[string]any{
			"items": []map[string]any{},
		}), status: fiber.StatusBadRequest},
		{name: "descuento mayor al subtotal", id: created.ID, body: jsonBody(t, map[string]any{
			"discount_amount": "1000",
		}), status: fiber.StatusBadRequest},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			status, body := doOrder(t, f.app, http.MethodPut, "/orders/"+caso.id, caso.body)
			if status != caso.status {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, caso.status, body)
			}
			errorMessage(t, body)
		})
	}
}

func TestDeleteOrderSoftDelete(t *testing.T) {
	f := newOrderAPIFixture(t)

	created := createOrderHTTP(t, f.app, map[string]any{
		"contact_id": f.contact.ID.String(),
		"items": []map[string]any{
			{"product_id": f.product.ID.String(), "quantity": 1},
		},
	})
	id := createdID(t, f, created.Code)

	status, body := doOrder(t, f.app, http.MethodDelete, "/orders/"+created.ID, "")
	if status != fiber.StatusNoContent {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusNoContent, body)
	}
	if strings.TrimSpace(body) != "" {
		t.Errorf("body = %q, se esperaba vacío", body)
	}

	// El borrado es lógico: cabecera y líneas quedan marcadas.
	if !f.orders.byID[id].DeletedAt.Valid {
		t.Error("la cabecera no quedó marcada como eliminada")
	}
	for _, item := range f.orders.items[id] {
		if !item.DeletedAt.Valid {
			t.Errorf("la línea %s no quedó eliminada", item.ID)
		}
	}

	// Y ya no se puede leer ni volver a borrar.
	if status, body := doOrder(t, f.app, http.MethodGet, "/orders/"+created.ID, ""); status != fiber.StatusNotFound {
		t.Errorf("get: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}
	if status, body := doOrder(t, f.app, http.MethodDelete, "/orders/"+created.ID, ""); status != fiber.StatusNotFound {
		t.Errorf("delete repetido: status = %d, se esperaba %d (body: %s)", status, fiber.StatusNotFound, body)
	}
}
