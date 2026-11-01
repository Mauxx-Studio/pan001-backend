package service

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

// fakeContactRepository es un repositorio en memoria para probar los casos de
// uso de terceros sin base de datos.
type fakeContactRepository struct {
	byID        map[uuid.UUID]*model.Contact
	lastFilters repository.ContactFilters
}

func newFakeContactRepository() *fakeContactRepository {
	return &fakeContactRepository{byID: map[uuid.UUID]*model.Contact{}}
}

func (r *fakeContactRepository) Create(_ context.Context, contact *model.Contact) error {
	if contact.ID == uuid.Nil {
		contact.ID = uuid.New()
	}
	now := time.Now().UTC()
	contact.CreatedAt, contact.UpdatedAt = now, now
	r.byID[contact.ID] = contact
	return nil
}

func (r *fakeContactRepository) FindByID(_ context.Context, id uuid.UUID) (*model.Contact, error) {
	contact, ok := r.byID[id]
	if !ok || contact.DeletedAt.Valid {
		return nil, model.ErrContactNotFound
	}
	return contact, nil
}

func (r *fakeContactRepository) List(_ context.Context, filters repository.ContactFilters) ([]model.Contact, error) {
	r.lastFilters = filters

	search := strings.ToLower(strings.TrimSpace(filters.Search))
	var contacts []model.Contact
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

	sort.Slice(contacts, func(i, j int) bool { return contacts[i].Name < contacts[j].Name })
	return contacts, nil
}

func (r *fakeContactRepository) Update(_ context.Context, contact *model.Contact) error {
	if _, ok := r.byID[contact.ID]; !ok {
		return model.ErrContactNotFound
	}
	contact.UpdatedAt = time.Now().UTC()
	r.byID[contact.ID] = contact
	return nil
}

func (r *fakeContactRepository) Delete(_ context.Context, id uuid.UUID) error {
	contact, ok := r.byID[id]
	if !ok || contact.DeletedAt.Valid {
		return model.ErrContactNotFound
	}
	contact.DeletedAt = gorm.DeletedAt{Time: time.Now().UTC(), Valid: true}
	return nil
}

func boolPtr(value bool) *bool { return &value }

func stringPtr(value string) *string { return &value }

func TestCreateContactAplicaValoresPorDefecto(t *testing.T) {
	repo := newFakeContactRepository()
	svc := NewContactService(repo)

	contact, err := svc.Create(context.Background(), CreateContactInput{
		Name:     "  Panadería El Sol  ",
		Email:    "  Contacto@ElSol.com ",
		Phone:    " 1122334455 ",
		WhatsApp: " 1122334455 ",
		City:     " CABA ",
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	if contact.Name != "Panadería El Sol" {
		t.Errorf("Name = %q, se esperaba el nombre sin espacios", contact.Name)
	}
	if contact.Email != "contacto@elsol.com" {
		t.Errorf("Email = %q, se esperaba el email normalizado", contact.Email)
	}
	if contact.Phone != "1122334455" || contact.City != "CABA" {
		t.Errorf("Phone/City = %q/%q, se esperaban los valores sin espacios", contact.Phone, contact.City)
	}
	if contact.ID == uuid.Nil {
		t.Error("ID = uuid.Nil, se esperaba un UUID generado")
	}
	if !contact.IsActive {
		t.Error("IsActive = false, se esperaba true por defecto")
	}
	if contact.IsClient || contact.IsSupplier {
		t.Errorf("IsClient/IsSupplier = %v/%v, se esperaba false/false por defecto", contact.IsClient, contact.IsSupplier)
	}
	if _, ok := repo.byID[contact.ID]; !ok {
		t.Error("el tercero no quedó guardado en el repositorio")
	}
}

func TestCreateContactClienteYProveedorALaVez(t *testing.T) {
	repo := newFakeContactRepository()
	svc := NewContactService(repo)

	contact, err := svc.Create(context.Background(), CreateContactInput{
		Name:         "Distribuidora Norte",
		BusinessName: "Norte SRL",
		IsClient:     boolPtr(true),
		IsSupplier:   boolPtr(true),
		IsActive:     boolPtr(false),
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	if !contact.IsClient || !contact.IsSupplier {
		t.Errorf("IsClient/IsSupplier = %v/%v, se esperaba true/true", contact.IsClient, contact.IsSupplier)
	}
	if contact.IsActive {
		t.Error("IsActive = true, se esperaba false cuando se envía explícitamente")
	}
}

func TestCreateContactSinNombre(t *testing.T) {
	repo := newFakeContactRepository()
	svc := NewContactService(repo)

	_, err := svc.Create(context.Background(), CreateContactInput{Name: "   "})
	if !errors.Is(err, model.ErrContactNameRequired) {
		t.Fatalf("error = %v, se esperaba model.ErrContactNameRequired", err)
	}
	if len(repo.byID) != 0 {
		t.Error("no debería haberse guardado ningún tercero")
	}
}

func TestGetContactNoEncontrado(t *testing.T) {
	svc := NewContactService(newFakeContactRepository())

	casos := map[string]uuid.UUID{
		"uuid inexistente": uuid.New(),
		"uuid nulo":        uuid.Nil,
	}

	for name, id := range casos {
		t.Run(name, func(t *testing.T) {
			_, err := svc.GetByID(context.Background(), id)
			if !errors.Is(err, model.ErrContactNotFound) {
				t.Fatalf("error = %v, se esperaba model.ErrContactNotFound", err)
			}
		})
	}
}

func TestListContactsFiltraYOrdena(t *testing.T) {
	repo := newFakeContactRepository()
	svc := NewContactService(repo)

	// Cliente activo, proveedor inactivo y un tercero sin rol definido.
	if _, err := svc.Create(context.Background(), CreateContactInput{
		Name:     "Zeta Cliente",
		IsClient: boolPtr(true),
	}); err != nil {
		t.Fatalf("no se pudo crear el tercero de prueba: %v", err)
	}
	if _, err := svc.Create(context.Background(), CreateContactInput{
		Name:       "Alfa Proveedor",
		IsSupplier: boolPtr(true),
		IsActive:   boolPtr(false),
	}); err != nil {
		t.Fatalf("no se pudo crear el tercero de prueba: %v", err)
	}
	if _, err := svc.Create(context.Background(), CreateContactInput{
		Name: "Beta Neutro",
	}); err != nil {
		t.Fatalf("no se pudo crear el tercero de prueba: %v", err)
	}

	contacts, err := svc.List(context.Background(), ListContactsInput{IsClient: boolPtr(true)})
	if err != nil {
		t.Fatalf("List() devolvió un error inesperado: %v", err)
	}
	if len(contacts) != 1 || contacts[0].Name != "Zeta Cliente" {
		t.Fatalf("List(is_client=true) = %+v, se esperaba solo Zeta Cliente", contacts)
	}

	// Sin filtros: orden alfabético por nombre.
	contacts, err = svc.List(context.Background(), ListContactsInput{})
	if err != nil {
		t.Fatalf("List() devolvió un error inesperado: %v", err)
	}
	if len(contacts) != 3 {
		t.Fatalf("List() devolvió %d terceros, se esperaban 3", len(contacts))
	}
	esperado := []string{"Alfa Proveedor", "Beta Neutro", "Zeta Cliente"}
	for i, name := range esperado {
		if contacts[i].Name != name {
			t.Errorf("contacts[%d].Name = %q, se esperaba %q", i, contacts[i].Name, name)
		}
	}

	// La búsqueda se recorta antes de llegar al repositorio.
	if _, err := svc.List(context.Background(), ListContactsInput{Search: "  alfa  "}); err != nil {
		t.Fatalf("List() devolvió un error inesperado: %v", err)
	}
	if repo.lastFilters.Search != "alfa" {
		t.Errorf("Search = %q, se esperaba el texto sin espacios", repo.lastFilters.Search)
	}
}

func TestListContactsVacioDevuelveSliceVacio(t *testing.T) {
	svc := NewContactService(newFakeContactRepository())

	contacts, err := svc.List(context.Background(), ListContactsInput{})
	if err != nil {
		t.Fatalf("List() devolvió un error inesperado: %v", err)
	}
	if contacts == nil {
		t.Error("List() devolvió nil, se esperaba un slice vacío")
	}
}

func TestUpdateContactParcial(t *testing.T) {
	repo := newFakeContactRepository()
	svc := NewContactService(repo)

	created, err := svc.Create(context.Background(), CreateContactInput{
		Name:       "Panadería El Sol",
		Phone:      "1122334455",
		City:       "CABA",
		IsClient:   boolPtr(true),
		IsSupplier: boolPtr(false),
	})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	updated, err := svc.Update(context.Background(), created.ID, UpdateContactInput{
		Phone:      stringPtr(" 1199998888 "),
		Email:      stringPtr(" VENTAS@ElSol.com "),
		IsSupplier: boolPtr(true),
	})
	if err != nil {
		t.Fatalf("Update() devolvió un error inesperado: %v", err)
	}

	if updated.Phone != "1199998888" {
		t.Errorf("Phone = %q, se esperaba el teléfono nuevo sin espacios", updated.Phone)
	}
	if updated.Email != "ventas@elsol.com" {
		t.Errorf("Email = %q, se esperaba el email normalizado", updated.Email)
	}
	if !updated.IsSupplier {
		t.Error("IsSupplier = false, se esperaba true")
	}
	// Los campos no enviados conservan su valor.
	if updated.Name != "Panadería El Sol" || updated.City != "CABA" || !updated.IsClient || !updated.IsActive {
		t.Errorf("campos no enviados modificados: %+v", updated)
	}
}

func TestUpdateContactNombreVacio(t *testing.T) {
	repo := newFakeContactRepository()
	svc := NewContactService(repo)

	created, err := svc.Create(context.Background(), CreateContactInput{Name: "Panadería El Sol"})
	if err != nil {
		t.Fatalf("Create() devolvió un error inesperado: %v", err)
	}

	_, err = svc.Update(context.Background(), created.ID, UpdateContactInput{Name: stringPtr("   ")})
	if !errors.Is(err, model.ErrContactNameRequired) {
		t.Fatalf("error = %v, se esperaba model.ErrContactNameRequired", err)
	}
	if repo.byID[created.ID].Name != "Panadería El Sol" {
		t.Error("el nombre guardado no debería haber cambiado")
	}
}

func TestUpdateContactNoEncontrado(t *testing.T) {
	svc := NewContactService(newFakeContactRepository())

	_, err := svc.Update(context.Background(), uuid.New(), UpdateContactInput{Name: stringPtr("Nuevo Nombre")})
	if !errors.Is(err, model.ErrContactNotFound) {
		t.Fatalf("error = %v, se esperaba model.ErrContactNotFound", err)
	}
}

func TestDeleteContactHaceSoftDelete(t *testing.T) {
	repo := newFakeContactRepository()
	svc := NewContactService(repo)

	created, err := svc.Create(context.Background(), CreateContactInput{Name: "Panadería El Sol"})
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
	if _, err := svc.GetByID(context.Background(), created.ID); !errors.Is(err, model.ErrContactNotFound) {
		t.Errorf("GetByID() después del delete = %v, se esperaba model.ErrContactNotFound", err)
	}

	// Borrar dos veces también es 404.
	if err := svc.Delete(context.Background(), created.ID); !errors.Is(err, model.ErrContactNotFound) {
		t.Errorf("segundo Delete() = %v, se esperaba model.ErrContactNotFound", err)
	}
}
