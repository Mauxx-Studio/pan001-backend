package service

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/repository"
)

// Valores por defecto del dominio de terceros.
const (
	// DefaultContactIsActive es el estado con el que se crea un tercero si el
	// cliente no envía is_active: los terceros nacen activos.
	DefaultContactIsActive = true
	// DefaultContactIsClient y DefaultContactIsSupplier son los valores por
	// defecto de los roles comerciales. Un tercero puede no ser ninguno de los
	// dos mientras se cargan sus datos.
	DefaultContactIsClient   = false
	DefaultContactIsSupplier = false
)

// CreateContactInput son los datos para dar de alta un tercero.
//
// Los booleanos son punteros para distinguir "no enviado" (nil) de false: el
// service resuelve los valores por defecto (is_active=true, is_client=false,
// is_supplier=false).
type CreateContactInput struct {
	Name         string
	BusinessName string
	TaxID        string
	Email        string
	Phone        string
	WhatsApp     string
	Address      string
	City         string
	Notes        string
	IsClient     *bool
	IsSupplier   *bool
	IsActive     *bool
}

// UpdateContactInput son los campos a modificar de un tercero. Un puntero en
// nil significa "no tocar": el endpoint PUT actualiza solo lo que se envía.
type UpdateContactInput struct {
	Name         *string
	BusinessName *string
	TaxID        *string
	Email        *string
	Phone        *string
	WhatsApp     *string
	Address      *string
	City         *string
	Notes        *string
	IsClient     *bool
	IsSupplier   *bool
	IsActive     *bool
}

// ListContactsInput son los filtros opcionales de listado. Los punteros en nil
// no filtran; Search busca en nombre, razón social, documento y contactos.
type ListContactsInput struct {
	IsClient   *bool
	IsSupplier *bool
	IsActive   *bool
	Search     string
}

// ContactService agrupa los casos de uso de terceros.
type ContactService struct {
	contacts repository.ContactRepository
}

// NewContactService construye el servicio de terceros.
func NewContactService(contacts repository.ContactRepository) *ContactService {
	return &ContactService{contacts: contacts}
}

// Create da de alta un tercero.
//
// Reglas:
//   - Name es el único campo obligatorio; se guarda sin espacios sobrantes.
//   - El email se normaliza (minúsculas y sin espacios), igual que en usuarios.
//   - is_client e is_supplier pueden ser ambos false, ambos true o combinados.
//   - is_active es true salvo que se indique lo contrario.
func (s *ContactService) Create(ctx context.Context, in CreateContactInput) (*model.Contact, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, model.ErrContactNameRequired
	}

	contact := &model.Contact{
		Name:         name,
		BusinessName: strings.TrimSpace(in.BusinessName),
		TaxID:        strings.TrimSpace(in.TaxID),
		Email:        model.NormalizeEmail(in.Email),
		Phone:        strings.TrimSpace(in.Phone),
		WhatsApp:     strings.TrimSpace(in.WhatsApp),
		Address:      strings.TrimSpace(in.Address),
		City:         strings.TrimSpace(in.City),
		Notes:        strings.TrimSpace(in.Notes),
		IsClient:     boolOr(in.IsClient, DefaultContactIsClient),
		IsSupplier:   boolOr(in.IsSupplier, DefaultContactIsSupplier),
		IsActive:     boolOr(in.IsActive, DefaultContactIsActive),
	}

	if err := s.contacts.Create(ctx, contact); err != nil {
		return nil, err
	}
	return contact, nil
}

// GetByID devuelve un tercero por su identificador.
//
// Reglas:
//   - Si no existe o fue soft-deleted → model.ErrContactNotFound.
func (s *ContactService) GetByID(ctx context.Context, id uuid.UUID) (*model.Contact, error) {
	if id == uuid.Nil {
		return nil, model.ErrContactNotFound
	}

	return s.contacts.FindByID(ctx, id)
}

// List devuelve los terceros no eliminados con los filtros indicados. El
// resultado nunca es nil, así la API responde [] y no null.
func (s *ContactService) List(ctx context.Context, in ListContactsInput) ([]model.Contact, error) {
	contacts, err := s.contacts.List(ctx, repository.ContactFilters{
		IsClient:   in.IsClient,
		IsSupplier: in.IsSupplier,
		IsActive:   in.IsActive,
		Search:     strings.TrimSpace(in.Search),
	})
	if err != nil {
		return nil, err
	}
	if contacts == nil {
		return []model.Contact{}, nil
	}
	return contacts, nil
}

// Update modifica solo los campos enviados (los nil se dejan como estaban).
//
// Reglas:
//   - Si el tercero no existe → model.ErrContactNotFound.
//   - Si se envía name, no puede quedar vacío → model.ErrContactNameRequired.
func (s *ContactService) Update(ctx context.Context, id uuid.UUID, in UpdateContactInput) (*model.Contact, error) {
	contact, err := s.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if in.Name != nil {
		name := strings.TrimSpace(*in.Name)
		if name == "" {
			return nil, model.ErrContactNameRequired
		}
		contact.Name = name
	}
	if in.BusinessName != nil {
		contact.BusinessName = strings.TrimSpace(*in.BusinessName)
	}
	if in.TaxID != nil {
		contact.TaxID = strings.TrimSpace(*in.TaxID)
	}
	if in.Email != nil {
		contact.Email = model.NormalizeEmail(*in.Email)
	}
	if in.Phone != nil {
		contact.Phone = strings.TrimSpace(*in.Phone)
	}
	if in.WhatsApp != nil {
		contact.WhatsApp = strings.TrimSpace(*in.WhatsApp)
	}
	if in.Address != nil {
		contact.Address = strings.TrimSpace(*in.Address)
	}
	if in.City != nil {
		contact.City = strings.TrimSpace(*in.City)
	}
	if in.Notes != nil {
		contact.Notes = strings.TrimSpace(*in.Notes)
	}
	if in.IsClient != nil {
		contact.IsClient = *in.IsClient
	}
	if in.IsSupplier != nil {
		contact.IsSupplier = *in.IsSupplier
	}
	if in.IsActive != nil {
		contact.IsActive = *in.IsActive
	}

	if err := s.contacts.Update(ctx, contact); err != nil {
		return nil, err
	}
	return contact, nil
}

// Delete hace un soft delete del tercero.
//
// Reglas:
//   - Si no existe o ya estaba eliminado → model.ErrContactNotFound.
func (s *ContactService) Delete(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return model.ErrContactNotFound
	}

	return s.contacts.Delete(ctx, id)
}

// boolOr devuelve el valor apuntado o el default cuando el campo no se envió.
func boolOr(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
