package service

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

// fakeOrderRepository es un repositorio de pedidos en memoria para probar los
// casos de uso sin base de datos.
//
// WithTransaction no simula rollback: la transacción real se prueba contra
// PostgreSQL. En memoria alcanza para verificar las reglas de negocio y que la
// cabecera y las líneas se guarden juntas.
type fakeOrderRepository struct {
	byID        map[uuid.UUID]*model.Order
	items       map[uuid.UUID][]*model.OrderItem
	lastFilters repository.OrderFilters

	// conflicts es cuántos Create todavía deben fallar con código duplicado.
	conflicts   int
	createCalls int
}

func newFakeOrderRepository() *fakeOrderRepository {
	return &fakeOrderRepository{
		byID:  map[uuid.UUID]*model.Order{},
		items: map[uuid.UUID][]*model.OrderItem{},
	}
}

func (r *fakeOrderRepository) Create(_ context.Context, order *model.Order) error {
	r.createCalls++
	if r.conflicts > 0 {
		r.conflicts--
		return model.ErrOrderCodeConflict
	}

	if order.ID == uuid.Nil {
		order.ID = uuid.New()
	}
	now := time.Now().UTC()
	order.CreatedAt, order.UpdatedAt = now, now

	items := make([]*model.OrderItem, 0, len(order.Items))
	for i := range order.Items {
		// Igual que GORM, el repositorio devuelve el id generado en el slice
		// que recibió: así el service puede responder con las líneas completas.
		if order.Items[i].ID == uuid.Nil {
			order.Items[i].ID = uuid.New()
		}
		order.Items[i].OrderID = order.ID
		order.Items[i].CreatedAt, order.Items[i].UpdatedAt = now, now
		items = append(items, cloneOrderItem(&order.Items[i], order.ID))
	}
	r.byID[order.ID] = cloneOrder(order)
	r.items[order.ID] = items
	return nil
}

func (r *fakeOrderRepository) FindByID(_ context.Context, id uuid.UUID) (*model.Order, error) {
	order, ok := r.byID[id]
	if !ok || order.DeletedAt.Valid {
		return nil, model.ErrOrderNotFound
	}

	found := cloneOrder(order)
	found.Items = make([]model.OrderItem, 0, len(r.items[id]))
	for _, item := range r.items[id] {
		if item.DeletedAt.Valid {
			continue
		}
		found.Items = append(found.Items, *cloneOrderItem(item, id))
	}
	return found, nil
}

func (r *fakeOrderRepository) List(_ context.Context, filters repository.OrderFilters) ([]model.Order, error) {
	r.lastFilters = filters

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
		orders = append(orders, *cloneOrder(order))
	}

	sort.Slice(orders, func(i, j int) bool { return orders[i].OrderedAt.After(orders[j].OrderedAt) })
	return orders, nil
}

func (r *fakeOrderRepository) Update(_ context.Context, order *model.Order) error {
	current, ok := r.byID[order.ID]
	if !ok || current.DeletedAt.Valid {
		return model.ErrOrderNotFound
	}

	updated := cloneOrder(order)
	updated.CreatedAt = current.CreatedAt
	updated.UpdatedAt = time.Now().UTC()
	r.byID[order.ID] = updated

	// Las líneas activas son las que el service dejó en order.Items: se copian
	// los valores mutables sobre las que ya estaban guardadas.
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

func (r *fakeOrderRepository) Delete(_ context.Context, id uuid.UUID) error {
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

func (r *fakeOrderRepository) NextDailySequence(_ context.Context, day time.Time) (int64, error) {
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

func (r *fakeOrderRepository) CreateItems(_ context.Context, items []model.OrderItem) error {
	now := time.Now().UTC()
	for i := range items {
		item := cloneOrderItem(&items[i], items[i].OrderID)
		if item.ID == uuid.Nil {
			item.ID = uuid.New()
		}
		item.CreatedAt, item.UpdatedAt = now, now
		r.items[item.OrderID] = append(r.items[item.OrderID], item)
	}
	return nil
}

func (r *fakeOrderRepository) UpdateItem(_ context.Context, item *model.OrderItem) error {
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

func (r *fakeOrderRepository) SoftDeleteItems(_ context.Context, orderID uuid.UUID, ids []uuid.UUID) error {
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

func (r *fakeOrderRepository) WithTransaction(ctx context.Context, fn func(tx repository.OrderRepository) error) error {
	return fn(r)
}

// fakeOrderRepository implementa repository.OrderRepository.
var _ repository.OrderRepository = (*fakeOrderRepository)(nil)

// cloneOrder copia la cabecera para que el servicio no comparta memoria con lo
// guardado en el repositorio falso.
func cloneOrder(order *model.Order) *model.Order {
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

// cloneOrderItem copia la línea asignándole el pedido al que pertenece.
func cloneOrderItem(item *model.OrderItem, orderID uuid.UUID) *model.OrderItem {
	copied := *item
	copied.OrderID = orderID
	return &copied
}

// orderTestFixture arma el servicio con un tercero y un producto ya creados,
// que es el escenario mínimo para dar de alta un pedido.
type orderTestFixture struct {
	svc      *OrderService
	orders   *fakeOrderRepository
	contacts *fakeContactRepository
	products *fakeProductRepository
	contact  *model.Contact
	product  *model.Product
}

func newOrderTestFixture(t *testing.T) *orderTestFixture {
	t.Helper()

	orderRepo := newFakeOrderRepository()
	contactRepo := newFakeContactRepository()
	productRepo := newFakeProductRepository()

	contact, err := NewContactService(contactRepo).Create(context.Background(), CreateContactInput{
		Name:     "Panadería Central",
		IsClient: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("creando tercero de prueba: %v", err)
	}

	product, err := NewProductService(productRepo).Create(context.Background(), CreateProductInput{
		Name:      "Pan francés",
		Unit:      "unidad",
		SalePrice: decimalPtr("150.50"),
	})
	if err != nil {
		t.Fatalf("creando producto de prueba: %v", err)
	}

	return &orderTestFixture{
		svc:      NewOrderService(orderRepo, contactRepo, productRepo),
		orders:   orderRepo,
		contacts: contactRepo,
		products: productRepo,
		contact:  contact,
		product:  product,
	}
}

// addProduct crea otro producto en el catálogo del fixture.
func (f *orderTestFixture) addProduct(t *testing.T, name, unit, salePrice string) *model.Product {
	t.Helper()

	product, err := NewProductService(f.products).Create(context.Background(), CreateProductInput{
		Name:      name,
		Unit:      unit,
		SalePrice: decimalPtr(salePrice),
	})
	if err != nil {
		t.Fatalf("creando producto %q: %v", name, err)
	}
	return product
}

// createOrder da de alta un pedido con una línea del producto base del fixture.
func (f *orderTestFixture) createOrder(t *testing.T, quantity string) *model.Order {
	t.Helper()

	order, err := f.svc.Create(context.Background(), CreateOrderInput{
		ContactID: f.contact.ID,
		Items: []CreateOrderItemInput{{
			ProductID: f.product.ID,
			Quantity:  decimal.RequireFromString(quantity),
		}},
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}
	return order
}

func TestCreateOrderCalculaTotalesConPrecioDelCatalogo(t *testing.T) {
	f := newOrderTestFixture(t)
	facturas := f.addProduct(t, "Facturas", "docena", "250")

	createdBy := uuid.New()
	order, err := f.svc.Create(context.Background(), CreateOrderInput{
		ContactID:      f.contact.ID,
		DeliveryType:   " Delivery ",
		PaymentMethod:  "CASH",
		DiscountAmount: decimalPtr("76.25"),
		AmountPaid:     decimalPtr("100"),
		Notes:          "  entregar temprano  ",
		CreatedBy:      &createdBy,
		Items: []CreateOrderItemInput{
			{ProductID: f.product.ID, Quantity: decimal.RequireFromString("2.5"), Notes: "  cortado fino  "},
			{ProductID: facturas.ID, Quantity: decimal.RequireFromString("2")},
		},
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	// El código se arma con el día del pedido y la primera secuencia libre.
	if want := model.NewOrderCode(order.OrderedAt, 1); order.Code != want {
		t.Errorf("code = %q, se esperaba %q", order.Code, want)
	}
	if order.Status != model.OrderStatusConfirmed {
		t.Errorf("status = %q, se esperaba %q", order.Status, model.OrderStatusConfirmed)
	}
	if order.PaymentStatus != model.PaymentStatusPending {
		t.Errorf("payment_status = %q, se esperaba %q", order.PaymentStatus, model.PaymentStatusPending)
	}
	if order.DeliveryType != model.DeliveryTypeDelivery || order.PaymentMethod != model.PaymentMethodCash {
		t.Errorf("delivery_type/payment_method = %q/%q, se esperaban delivery/cash",
			order.DeliveryType, order.PaymentMethod)
	}
	if order.Notes != "entregar temprano" {
		t.Errorf("notes = %q, se esperaba recortado", order.Notes)
	}
	if !order.IsActive {
		t.Error("IsActive = false, se esperaba true por defecto")
	}
	if order.CreatedBy == nil || *order.CreatedBy != createdBy {
		t.Errorf("created_by = %v, se esperaba %s", order.CreatedBy, createdBy)
	}

	// 2.5 * 150.50 + 2 * 250 = 376.25 + 500 = 876.25; con el descuento, 800.
	assertDecimalEqual(t, "subtotal", order.Subtotal, "876.25")
	assertDecimalEqual(t, "discount_amount", order.DiscountAmount, "76.25")
	assertDecimalEqual(t, "final_amount", order.FinalAmount, "800")
	assertDecimalEqual(t, "amount_paid", order.AmountPaid, "100")

	details, err := f.svc.GetByID(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("GetByID() devolvió un error inesperado: %v", err)
	}
	if len(details.Items) != 2 {
		t.Fatalf("len(Items) = %d, se esperaban 2", len(details.Items))
	}

	first := details.Items[0]
	if first.ProductName != "Pan francés" || first.Unit != "unidad" {
		t.Errorf("snapshot del producto = %q/%q, se esperaban Pan francés/unidad",
			first.ProductName, first.Unit)
	}
	assertDecimalEqual(t, "quantity", first.Quantity, "2.5")
	assertDecimalEqual(t, "unit_price", first.UnitPrice, "150.5")
	assertDecimalEqual(t, "line_total", first.LineTotal, "376.25")
	assertDecimalEqual(t, "quantity_delivered", first.QuantityDelivered, "0")
	if first.Notes != "cortado fino" {
		t.Errorf("notes de la línea = %q, se esperaba recortado", first.Notes)
	}
	if first.ID == uuid.Nil || first.OrderID != order.ID {
		t.Errorf("la línea no quedó asociada al pedido: %+v", first)
	}

	assertDecimalEqual(t, "subtotal recalculado", details.Subtotal, "876.25")
	assertDecimalEqual(t, "final_amount recalculado", details.FinalAmount, "800")
}

func TestCreateOrderGeneraCodigosConsecutivos(t *testing.T) {
	f := newOrderTestFixture(t)

	first := f.createOrder(t, "1")
	second := f.createOrder(t, "1")

	if want := model.NewOrderCode(first.OrderedAt, 1); first.Code != want {
		t.Errorf("primer code = %q, se esperaba %q", first.Code, want)
	}
	if want := model.NewOrderCode(second.OrderedAt, 2); second.Code != want {
		t.Errorf("segundo code = %q, se esperaba %q", second.Code, want)
	}
	if first.Code == second.Code {
		t.Error("dos pedidos no pueden compartir el código")
	}
}

func TestCreateOrderBorradorPuedeQuedarVacio(t *testing.T) {
	f := newOrderTestFixture(t)

	order, err := f.svc.Create(context.Background(), CreateOrderInput{
		ContactID: f.contact.ID,
		Status:    " Draft ",
	})
	if err != nil {
		t.Fatalf("Create() de un borrador vacío devolvió un error inesperado: %v", err)
	}
	if order.Status != model.OrderStatusDraft {
		t.Errorf("status = %q, se esperaba %q", order.Status, model.OrderStatusDraft)
	}
	if len(order.Items) != 0 {
		t.Errorf("len(Items) = %d, se esperaba 0", len(order.Items))
	}
	assertDecimalEqual(t, "subtotal", order.Subtotal, "0")
	assertDecimalEqual(t, "final_amount", order.FinalAmount, "0")

	// Un pedido vacío que no es borrador se rechaza.
	_, err = f.svc.Create(context.Background(), CreateOrderInput{ContactID: f.contact.ID})
	if !errors.Is(err, model.ErrOrderItemsRequired) {
		t.Fatalf("error = %v, se esperaba model.ErrOrderItemsRequired", err)
	}
}

func TestCreateOrderValidaDatos(t *testing.T) {
	f := newOrderTestFixture(t)
	singleLine := func(productID uuid.UUID, quantity string) []CreateOrderItemInput {
		return []CreateOrderItemInput{{
			ProductID: productID,
			Quantity:  decimal.RequireFromString(quantity),
		}}
	}

	// Producto con precio de venta en cero: no se puede cotizar.
	freeProduct, err := NewProductService(f.products).Create(context.Background(), CreateProductInput{
		Name:      "Sin precio",
		Unit:      "unidad",
		SalePrice: decimalPtr("0"),
	})
	if err != nil {
		t.Fatalf("creando producto sin precio: %v", err)
	}

	casos := []struct {
		name string
		in   CreateOrderInput
		want error
	}{
		{
			name: "sin tercero",
			in:   CreateOrderInput{Items: singleLine(f.product.ID, "1")},
			want: model.ErrOrderContactRequired,
		},
		{
			name: "tercero inexistente",
			in:   CreateOrderInput{ContactID: uuid.New(), Items: singleLine(f.product.ID, "1")},
			want: model.ErrContactNotFound,
		},
		{
			name: "estado inválido",
			in:   CreateOrderInput{ContactID: f.contact.ID, Status: "en_camino", Items: singleLine(f.product.ID, "1")},
			want: model.ErrOrderStatusInvalid,
		},
		{
			name: "tipo de entrega inválido",
			in:   CreateOrderInput{ContactID: f.contact.ID, DeliveryType: "drone", Items: singleLine(f.product.ID, "1")},
			want: model.ErrOrderDeliveryTypeInvalid,
		},
		{
			name: "medio de pago inválido",
			in:   CreateOrderInput{ContactID: f.contact.ID, PaymentMethod: "bitcoin", Items: singleLine(f.product.ID, "1")},
			want: model.ErrOrderPaymentMethodInvalid,
		},
		{
			name: "estado de pago inválido",
			in:   CreateOrderInput{ContactID: f.contact.ID, PaymentStatus: "cobrado", Items: singleLine(f.product.ID, "1")},
			want: model.ErrOrderPaymentStatusInvalid,
		},
		{
			name: "producto inexistente",
			in:   CreateOrderInput{ContactID: f.contact.ID, Items: singleLine(uuid.New(), "1")},
			want: model.ErrProductNotFound,
		},
		{
			name: "producto sin precio",
			in:   CreateOrderInput{ContactID: f.contact.ID, Items: singleLine(freeProduct.ID, "1")},
			want: model.ErrOrderProductPriceInvalid,
		},
		{
			name: "producto vacío",
			in:   CreateOrderInput{ContactID: f.contact.ID, Items: singleLine(uuid.Nil, "1")},
			want: model.ErrOrderItemProductRequired,
		},
		{
			name: "cantidad cero",
			in:   CreateOrderInput{ContactID: f.contact.ID, Items: singleLine(f.product.ID, "0")},
			want: model.ErrOrderItemQuantityInvalid,
		},
		{
			name: "cantidad negativa",
			in:   CreateOrderInput{ContactID: f.contact.ID, Items: singleLine(f.product.ID, "-3")},
			want: model.ErrOrderItemQuantityInvalid,
		},
		{
			name: "descuento negativo",
			in: CreateOrderInput{
				ContactID:      f.contact.ID,
				DiscountAmount: decimalPtr("-1"),
				Items:          singleLine(f.product.ID, "1"),
			},
			want: model.ErrOrderDiscountInvalid,
		},
		{
			name: "descuento mayor al subtotal",
			in: CreateOrderInput{
				ContactID:      f.contact.ID,
				DiscountAmount: decimalPtr("9999"),
				Items:          singleLine(f.product.ID, "1"),
			},
			want: model.ErrOrderDiscountInvalid,
		},
		{
			name: "importe cobrado negativo",
			in: CreateOrderInput{
				ContactID:  f.contact.ID,
				AmountPaid: decimalPtr("-0.01"),
				Items:      singleLine(f.product.ID, "1"),
			},
			want: model.ErrOrderAmountPaidInvalid,
		},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			if _, err := f.svc.Create(context.Background(), caso.in); !errors.Is(err, caso.want) {
				t.Fatalf("error = %v, se esperaba %v", err, caso.want)
			}
		})
	}

	// Nada de lo anterior se guardó: el fixture sigue sin pedidos.
	if len(f.orders.byID) != 0 {
		t.Errorf("se guardaron %d pedidos inválidos", len(f.orders.byID))
	}
}

func TestCreateOrderReintentaCuandoElCodigoSeRepite(t *testing.T) {
	f := newOrderTestFixture(t)
	// Las dos primeras inserciones chocan con el índice único de code.
	f.orders.conflicts = 2

	order := f.createOrder(t, "1")

	if f.orders.createCalls != 3 {
		t.Errorf("Create del repositorio se llamó %d veces, se esperaban 3", f.orders.createCalls)
	}
	if want := model.NewOrderCode(order.OrderedAt, 1); order.Code != want {
		t.Errorf("code = %q, se esperaba %q", order.Code, want)
	}

	// Si la secuencia nunca se libera, el alta falla con un error de conflicto.
	otros := newOrderTestFixture(t)
	otros.orders.conflicts = 5
	_, err := otros.svc.Create(context.Background(), CreateOrderInput{
		ContactID: otros.contact.ID,
		Items: []CreateOrderItemInput{{
			ProductID: otros.product.ID,
			Quantity:  decimal.RequireFromString("1"),
		}},
	})
	if !errors.Is(err, model.ErrOrderCodeConflict) {
		t.Fatalf("error = %v, se esperaba model.ErrOrderCodeConflict", err)
	}
}

// assertDecimalEqual compara dos decimales por valor (no por representación).
func assertDecimalEqual(t *testing.T, field string, got decimal.Decimal, want string) {
	t.Helper()

	expected := decimal.RequireFromString(want)
	if !got.Equal(expected) {
		t.Errorf("%s = %s, se esperaba %s", field, got, expected)
	}
}

func TestGetByIDPedidoInexistente(t *testing.T) {
	f := newOrderTestFixture(t)

	if _, err := f.svc.GetByID(context.Background(), uuid.Nil); !errors.Is(err, model.ErrOrderNotFound) {
		t.Errorf("uuid.Nil: error = %v, se esperaba model.ErrOrderNotFound", err)
	}
	if _, err := f.svc.GetByID(context.Background(), uuid.New()); !errors.Is(err, model.ErrOrderNotFound) {
		t.Errorf("uuid desconocido: error = %v, se esperaba model.ErrOrderNotFound", err)
	}
}

func TestDeleteOrderHaceSoftDelete(t *testing.T) {
	f := newOrderTestFixture(t)
	order := f.createOrder(t, "1")

	if err := f.svc.Delete(context.Background(), order.ID); err != nil {
		t.Fatalf("Delete() devolvió un error inesperado: %v", err)
	}
	if _, err := f.svc.GetByID(context.Background(), order.ID); !errors.Is(err, model.ErrOrderNotFound) {
		t.Errorf("después de Delete, GetByID() error = %v, se esperaba model.ErrOrderNotFound", err)
	}
	// Las líneas se van con la cabecera: no quedan pedidos "fantasma".
	if len(f.orders.items[order.ID]) != 1 || !f.orders.items[order.ID][0].DeletedAt.Valid {
		t.Errorf("las líneas del pedido no quedaron eliminadas: %+v", f.orders.items[order.ID])
	}
	if err := f.svc.Delete(context.Background(), order.ID); !errors.Is(err, model.ErrOrderNotFound) {
		t.Errorf("segundo Delete(): error = %v, se esperaba model.ErrOrderNotFound", err)
	}
	if err := f.svc.Delete(context.Background(), uuid.Nil); !errors.Is(err, model.ErrOrderNotFound) {
		t.Errorf("Delete(uuid.Nil): error = %v, se esperaba model.ErrOrderNotFound", err)
	}
}

func TestUpdateOrderReemplazaLineasYRecalculaTotales(t *testing.T) {
	f := newOrderTestFixture(t)
	facturas := f.addProduct(t, "Facturas", "docena", "250")
	order := f.createOrder(t, "2") // 2 * 150.50 = 301.00
	existingID := order.Items[0].ID

	// Se sube la cantidad de la línea existente y se agrega otra: el conjunto
	// enviado es el que queda.
	items := []UpdateOrderItemInput{
		{ID: &existingID, Quantity: decimalPtr("3")},
		{ProductID: &facturas.ID, Quantity: decimalPtr("1"), Notes: stringPtr("  con crema  ")},
	}
	updated, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{Items: &items})
	if err != nil {
		t.Fatalf("Update() devolvió un error inesperado: %v", err)
	}

	if len(updated.Items) != 2 {
		t.Fatalf("len(Items) = %d, se esperaban 2", len(updated.Items))
	}
	// 3 * 150.50 + 250 = 451.50 + 250 = 701.50
	assertDecimalEqual(t, "subtotal", updated.Subtotal, "701.5")
	assertDecimalEqual(t, "final_amount", updated.FinalAmount, "701.5")

	var facturasLine *model.OrderItem
	for i := range updated.Items {
		if updated.Items[i].ProductID == facturas.ID {
			facturasLine = &updated.Items[i]
		}
	}
	if facturasLine == nil {
		t.Fatal("la línea nueva no quedó guardada")
	}
	if facturasLine.ID == uuid.Nil {
		t.Error("la línea nueva se guardó sin id")
	}
	if facturasLine.ProductName != "Facturas" || facturasLine.Unit != "docena" {
		t.Errorf("snapshot = %q/%q, se esperaban Facturas/docena", facturasLine.ProductName, facturasLine.Unit)
	}
	assertDecimalEqual(t, "unit_price", facturasLine.UnitPrice, "250")
	assertDecimalEqual(t, "line_total", facturasLine.LineTotal, "250")
	if facturasLine.Notes != "con crema" {
		t.Errorf("notes = %q, se esperaba recortado", facturasLine.Notes)
	}

	// items ausente deja las líneas como están.
	notes := "sin tocar líneas"
	updated, err = f.svc.Update(context.Background(), order.ID, UpdateOrderInput{Notes: &notes})
	if err != nil {
		t.Fatalf("Update() devolvió un error inesperado: %v", err)
	}
	if len(updated.Items) != 2 {
		t.Errorf("len(Items) = %d, se esperaban 2 (items en nil)", len(updated.Items))
	}
	if updated.Notes != notes {
		t.Errorf("notes = %q, se esperaba %q", updated.Notes, notes)
	}
}

func TestUpdateOrderValidaLineas(t *testing.T) {
	f := newOrderTestFixture(t)
	facturas := f.addProduct(t, "Facturas", "docena", "250")
	order := f.createOrder(t, "2")
	other := f.createOrder(t, "1")
	existingID := order.Items[0].ID
	foreignID := other.Items[0].ID
	unknownProduct := uuid.New()

	// El service ignora quantity_delivered del body: para simular una entrega
	// parcial ya registrada se modifica lo guardado.
	f.orders.items[order.ID][0].QuantityDelivered = decimal.RequireFromString("1")

	casos := []struct {
		name  string
		items []UpdateOrderItemInput
		want  error
	}{
		{
			name:  "quitar una línea con entregas",
			items: []UpdateOrderItemInput{{ProductID: &facturas.ID, Quantity: decimalPtr("1")}},
			want:  model.ErrOrderItemDeliveredLocked,
		},
		{
			name:  "bajar la cantidad por debajo de lo entregado",
			items: []UpdateOrderItemInput{{ID: &existingID, Quantity: decimalPtr("0.5")}},
			want:  model.ErrOrderItemDeliveredInvalid,
		},
		{
			name:  "línea de otro pedido",
			items: []UpdateOrderItemInput{{ID: &foreignID, Quantity: decimalPtr("1")}},
			want:  model.ErrOrderItemNotInOrder,
		},
		{
			name: "línea repetida",
			items: []UpdateOrderItemInput{
				{ID: &existingID, Quantity: decimalPtr("2")},
				{ID: &existingID, Notes: stringPtr("otra vez")},
			},
			want: model.ErrOrderItemDuplicated,
		},
		{
			name:  "línea nueva sin producto",
			items: []UpdateOrderItemInput{{Quantity: decimalPtr("1")}},
			want:  model.ErrOrderItemProductRequired,
		},
		{
			name:  "línea nueva sin cantidad",
			items: []UpdateOrderItemInput{{ProductID: &facturas.ID}},
			want:  model.ErrOrderItemQuantityInvalid,
		},
		{
			name:  "línea nueva con producto inexistente",
			items: []UpdateOrderItemInput{{ProductID: &unknownProduct, Quantity: decimalPtr("1")}},
			want:  model.ErrProductNotFound,
		},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			items := caso.items
			if _, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{Items: &items}); !errors.Is(err, caso.want) {
				t.Fatalf("error = %v, se esperaba %v", err, caso.want)
			}
		})
	}

	// Subir la cantidad de una línea ya entregada sí se permite.
	items := []UpdateOrderItemInput{{ID: &existingID, Quantity: decimalPtr("4"), Notes: stringPtr("  urgente  ")}}
	updated, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{Items: &items})
	if err != nil {
		t.Fatalf("Update() devolvió un error inesperado: %v", err)
	}
	if len(updated.Items) != 1 {
		t.Fatalf("len(Items) = %d, se esperaba 1", len(updated.Items))
	}
	// 4 * 150.50 = 602.00
	assertDecimalEqual(t, "subtotal", updated.Subtotal, "602")
	assertDecimalEqual(t, "quantity_delivered", updated.Items[0].QuantityDelivered, "1")
	if updated.Items[0].Notes != "urgente" {
		t.Errorf("notes = %q, se esperaba recortado", updated.Items[0].Notes)
	}
}

func TestUpdateOrderValidaCabecera(t *testing.T) {
	f := newOrderTestFixture(t)
	order := f.createOrder(t, "1")

	if _, err := f.svc.Update(context.Background(), uuid.New(), UpdateOrderInput{}); !errors.Is(err, model.ErrOrderNotFound) {
		t.Errorf("pedido inexistente: error = %v, se esperaba model.ErrOrderNotFound", err)
	}
	if _, err := f.svc.Update(context.Background(), uuid.Nil, UpdateOrderInput{}); !errors.Is(err, model.ErrOrderNotFound) {
		t.Errorf("uuid.Nil: error = %v, se esperaba model.ErrOrderNotFound", err)
	}

	nilContact := uuid.Nil
	if _, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{ContactID: &nilContact}); !errors.Is(err, model.ErrOrderContactRequired) {
		t.Errorf("contact vacío: error = %v, se esperaba model.ErrOrderContactRequired", err)
	}
	unknownContact := uuid.New()
	if _, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{ContactID: &unknownContact}); !errors.Is(err, model.ErrContactNotFound) {
		t.Errorf("contact inexistente: error = %v, se esperaba model.ErrContactNotFound", err)
	}
	badStatus := "en_camino"
	if _, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{Status: &badStatus}); !errors.Is(err, model.ErrOrderStatusInvalid) {
		t.Errorf("status inválido: error = %v, se esperaba model.ErrOrderStatusInvalid", err)
	}
	emptyStatus := ""
	if _, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{Status: &emptyStatus}); !errors.Is(err, model.ErrOrderStatusInvalid) {
		t.Errorf("status vacío: error = %v, se esperaba model.ErrOrderStatusInvalid", err)
	}
	bigDiscount := decimalPtr("9999")
	if _, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{DiscountAmount: bigDiscount}); !errors.Is(err, model.ErrOrderDiscountInvalid) {
		t.Errorf("descuento mayor al subtotal: error = %v, se esperaba model.ErrOrderDiscountInvalid", err)
	}
	negativePaid := decimalPtr("-1")
	if _, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{AmountPaid: negativePaid}); !errors.Is(err, model.ErrOrderAmountPaidInvalid) {
		t.Errorf("importe cobrado negativo: error = %v, se esperaba model.ErrOrderAmountPaidInvalid", err)
	}

	// Una línea conserva su estado: el pedido queda intacto después de los
	// intentos inválidos.
	current, err := f.svc.GetByID(context.Background(), order.ID)
	if err != nil {
		t.Fatalf("GetByID() devolvió un error inesperado: %v", err)
	}
	if current.Status != model.OrderStatusConfirmed || current.ContactID != f.contact.ID {
		t.Errorf("el pedido cambió después de updates inválidos: %+v", current)
	}
	assertDecimalEqual(t, "subtotal", current.Subtotal, "150.5")
}

func TestUpdateOrderBorraLineasQueNoVienenEnElBody(t *testing.T) {
	f := newOrderTestFixture(t)
	facturas := f.addProduct(t, "Facturas", "docena", "250")
	order := f.createOrder(t, "2")

	// Solo se envía la línea de facturas: la del producto base desaparece.
	items := []UpdateOrderItemInput{{ProductID: &facturas.ID, Quantity: decimalPtr("1")}}
	updated, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{Items: &items})
	if err != nil {
		t.Fatalf("Update() devolvió un error inesperado: %v", err)
	}
	if len(updated.Items) != 1 || updated.Items[0].ProductID != facturas.ID {
		t.Fatalf("Items = %+v, se esperaba solo la línea de Facturas", updated.Items)
	}
	assertDecimalEqual(t, "subtotal", updated.Subtotal, "250")

	// Vaciar las líneas de un pedido que no es borrador no es válido.
	empty := []UpdateOrderItemInput{}
	_, err = f.svc.Update(context.Background(), order.ID, UpdateOrderInput{Items: &empty})
	if !errors.Is(err, model.ErrOrderItemsRequired) {
		t.Fatalf("error = %v, se esperaba model.ErrOrderItemsRequired", err)
	}

	// En un borrador sí se admite.
	draft, err := f.svc.Create(context.Background(), CreateOrderInput{
		ContactID: f.contact.ID,
		Status:    model.OrderStatusDraft,
		Items:     []CreateOrderItemInput{{ProductID: facturas.ID, Quantity: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create() de borrador devolvió un error inesperado: %v", err)
	}
	draftItems := []UpdateOrderItemInput{}
	emptied, err := f.svc.Update(context.Background(), draft.ID, UpdateOrderInput{Items: &draftItems})
	if err != nil {
		t.Fatalf("Update() de borrador devolvió un error inesperado: %v", err)
	}
	if len(emptied.Items) != 0 {
		t.Errorf("len(Items) = %d, se esperaba 0", len(emptied.Items))
	}
	assertDecimalEqual(t, "final_amount", emptied.FinalAmount, "0")
}

func TestUpdateOrderPermiteVaciarCamposOpcionales(t *testing.T) {
	f := newOrderTestFixture(t)
	deliveryAt := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)

	order, err := f.svc.Create(context.Background(), CreateOrderInput{
		ContactID:     f.contact.ID,
		DeliveryAt:    &deliveryAt,
		DeliveryType:  model.DeliveryTypeDelivery,
		PaymentMethod: model.PaymentMethodTransfer,
		Items:         []CreateOrderItemInput{{ProductID: f.product.ID, Quantity: decimal.RequireFromString("1")}},
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}
	if order.DeliveryAt == nil || !order.DeliveryAt.Equal(deliveryAt) {
		t.Fatalf("delivery_at = %v, se esperaba %v", order.DeliveryAt, deliveryAt)
	}

	// "" limpia delivery_type y payment_method; la fecha cero limpia delivery_at.
	empty := ""
	zero := time.Time{}
	updated, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{
		DeliveryType:  &empty,
		PaymentMethod: &empty,
		DeliveryAt:    &zero,
	})
	if err != nil {
		t.Fatalf("Update() devolvió un error inesperado: %v", err)
	}
	if updated.DeliveryType != "" || updated.PaymentMethod != "" {
		t.Errorf("delivery_type/payment_method = %q/%q, se esperaban vacíos",
			updated.DeliveryType, updated.PaymentMethod)
	}
	if updated.DeliveryAt != nil {
		t.Errorf("delivery_at = %v, se esperaba nil", updated.DeliveryAt)
	}

	// Un tipo de entrega inválido se rechaza.
	bad := "drone"
	if _, err := f.svc.Update(context.Background(), order.ID, UpdateOrderInput{DeliveryType: &bad}); !errors.Is(err, model.ErrOrderDeliveryTypeInvalid) {
		t.Errorf("error = %v, se esperaba model.ErrOrderDeliveryTypeInvalid", err)
	}
}

func TestListOrdersFiltra(t *testing.T) {
	f := newOrderTestFixture(t)
	first := f.createOrder(t, "1")
	second := f.createOrder(t, "2")

	status := model.OrderStatusInProduction
	paymentStatus := model.PaymentStatusPaid
	notes := "pedido grande"
	if _, err := f.svc.Update(context.Background(), second.ID, UpdateOrderInput{
		Status:        &status,
		PaymentStatus: &paymentStatus,
		Notes:         &notes,
	}); err != nil {
		t.Fatalf("Update() devolvió un error inesperado: %v", err)
	}

	todos, err := f.svc.List(context.Background(), ListOrdersInput{})
	if err != nil {
		t.Fatalf("List() devolvió un error inesperado: %v", err)
	}
	if len(todos) != 2 {
		t.Fatalf("len(orders) = %d, se esperaban 2", len(todos))
	}
	// El listado viene del más nuevo al más viejo.
	if todos[0].ID != second.ID || todos[1].ID != first.ID {
		t.Errorf("orden inesperado: primero %s, segundo %s", todos[0].ID, todos[1].ID)
	}

	casos := []struct {
		name    string
		in      ListOrdersInput
		wantIDs []uuid.UUID
	}{
		{
			name:    "por estado normalizado",
			in:      ListOrdersInput{Status: " In_Production "},
			wantIDs: []uuid.UUID{second.ID},
		},
		{
			name:    "por estado de pago",
			in:      ListOrdersInput{PaymentStatus: "PAID"},
			wantIDs: []uuid.UUID{second.ID},
		},
		{
			name:    "por tercero",
			in:      ListOrdersInput{ContactID: &f.contact.ID},
			wantIDs: []uuid.UUID{second.ID, first.ID},
		},
		{
			name:    "por búsqueda en el código",
			in:      ListOrdersInput{Search: strings.ToLower(second.Code)},
			wantIDs: []uuid.UUID{second.ID},
		},
		{
			name:    "por búsqueda en las notas",
			in:      ListOrdersInput{Search: "  GRANDE  "},
			wantIDs: []uuid.UUID{second.ID},
		},
		{
			name:    "rango que no incluye nada",
			in:      ListOrdersInput{To: timePtr(time.Now().UTC().Add(-48 * time.Hour))},
			wantIDs: []uuid.UUID{},
		},
	}

	for _, caso := range casos {
		t.Run(caso.name, func(t *testing.T) {
			orders, err := f.svc.List(context.Background(), caso.in)
			if err != nil {
				t.Fatalf("List() devolvió un error inesperado: %v", err)
			}
			if orders == nil {
				t.Fatal("List() devolvió nil, se esperaba un slice vacío")
			}
			if len(orders) != len(caso.wantIDs) {
				t.Fatalf("len(orders) = %d, se esperaban %d: %+v", len(orders), len(caso.wantIDs), orders)
			}
			for i := range caso.wantIDs {
				if orders[i].ID != caso.wantIDs[i] {
					t.Errorf("orders[%d].ID = %s, se esperaba %s", i, orders[i].ID, caso.wantIDs[i])
				}
			}
		})
	}

	// Un tercero sin pedidos devuelve lista vacía, no nil.
	otroContacto, err := NewContactService(f.contacts).Create(context.Background(), CreateContactInput{
		Name:     "Almacén Sur",
		IsClient: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("creando otro tercero: %v", err)
	}
	vacio, err := f.svc.List(context.Background(), ListOrdersInput{ContactID: &otroContacto.ID})
	if err != nil {
		t.Fatalf("List() devolvió un error inesperado: %v", err)
	}
	if vacio == nil || len(vacio) != 0 {
		t.Errorf("orders = %v, se esperaba una lista vacía no nil", vacio)
	}

	// Los enums inválidos se rechazan.
	if _, err := f.svc.List(context.Background(), ListOrdersInput{Status: "en_camino"}); !errors.Is(err, model.ErrOrderStatusInvalid) {
		t.Errorf("status inválido: error = %v, se esperaba model.ErrOrderStatusInvalid", err)
	}
	if _, err := f.svc.List(context.Background(), ListOrdersInput{PaymentStatus: "cobrado"}); !errors.Is(err, model.ErrOrderPaymentStatusInvalid) {
		t.Errorf("payment_status inválido: error = %v, se esperaba model.ErrOrderPaymentStatusInvalid", err)
	}

	// Un pedido eliminado deja de aparecer.
	if err := f.svc.Delete(context.Background(), first.ID); err != nil {
		t.Fatalf("Delete() devolvió un error inesperado: %v", err)
	}
	restantes, err := f.svc.List(context.Background(), ListOrdersInput{})
	if err != nil {
		t.Fatalf("List() devolvió un error inesperado: %v", err)
	}
	if len(restantes) != 1 || restantes[0].ID != second.ID {
		t.Errorf("orders = %+v, se esperaba solo el segundo pedido", restantes)
	}
}

// timePtr devuelve un puntero a la fecha recibida.
func timePtr(value time.Time) *time.Time { return &value }
