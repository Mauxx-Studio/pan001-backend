package handler

import (
	"time"

	"github.com/pan001/backend/internal/model"
)

// createContactRequest es el body de POST /api/v1/contacts.
//
// Los booleanos son punteros para distinguir "no enviado" de false: el service
// resuelve is_active=true, is_client=false e is_supplier=false por defecto.
type createContactRequest struct {
	Name         string `json:"name" validate:"required,max=150"`
	BusinessName string `json:"business_name" validate:"omitempty,max=200"`
	TaxID        string `json:"tax_id" validate:"omitempty,max=30"`
	Email        string `json:"email" validate:"omitempty,email,max=255"`
	Phone        string `json:"phone" validate:"omitempty,max=30"`
	WhatsApp     string `json:"whatsapp" validate:"omitempty,max=30"`
	Address      string `json:"address" validate:"omitempty,max=255"`
	City         string `json:"city" validate:"omitempty,max=100"`
	Notes        string `json:"notes" validate:"omitempty,max=2000"`
	IsClient     *bool  `json:"is_client"`
	IsSupplier   *bool  `json:"is_supplier"`
	IsActive     *bool  `json:"is_active"`
}

// updateContactRequest es el body de PUT /api/v1/contacts/:id.
//
// Todos los campos son opcionales: solo se actualizan los punteros no nulos, de
// modo que un cliente puede enviar únicamente lo que cambia. name, si viene, no
// puede quedar vacío (lo valida el service después de recortar espacios).
type updateContactRequest struct {
	Name         *string `json:"name" validate:"omitempty,max=150"`
	BusinessName *string `json:"business_name" validate:"omitempty,max=200"`
	TaxID        *string `json:"tax_id" validate:"omitempty,max=30"`
	Email        *string `json:"email" validate:"omitempty,email,max=255"`
	Phone        *string `json:"phone" validate:"omitempty,max=30"`
	WhatsApp     *string `json:"whatsapp" validate:"omitempty,max=30"`
	Address      *string `json:"address" validate:"omitempty,max=255"`
	City         *string `json:"city" validate:"omitempty,max=100"`
	Notes        *string `json:"notes" validate:"omitempty,max=2000"`
	IsClient     *bool   `json:"is_client"`
	IsSupplier   *bool   `json:"is_supplier"`
	IsActive     *bool   `json:"is_active"`
}

// contactResponse es la representación pública de un tercero.
type contactResponse struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	BusinessName string    `json:"business_name,omitempty"`
	TaxID        string    `json:"tax_id,omitempty"`
	Email        string    `json:"email,omitempty"`
	Phone        string    `json:"phone,omitempty"`
	WhatsApp     string    `json:"whatsapp,omitempty"`
	Address      string    `json:"address,omitempty"`
	City         string    `json:"city,omitempty"`
	Notes        string    `json:"notes,omitempty"`
	IsClient     bool      `json:"is_client"`
	IsSupplier   bool      `json:"is_supplier"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// newContactResponse mapea la entidad de dominio al DTO de salida. Las fechas se
// normalizan a UTC, igual que en las respuestas de usuarios.
func newContactResponse(contact *model.Contact) contactResponse {
	return contactResponse{
		ID:           contact.ID.String(),
		Name:         contact.Name,
		BusinessName: contact.BusinessName,
		TaxID:        contact.TaxID,
		Email:        contact.Email,
		Phone:        contact.Phone,
		WhatsApp:     contact.WhatsApp,
		Address:      contact.Address,
		City:         contact.City,
		Notes:        contact.Notes,
		IsClient:     contact.IsClient,
		IsSupplier:   contact.IsSupplier,
		IsActive:     contact.IsActive,
		CreatedAt:    contact.CreatedAt.UTC(),
		UpdatedAt:    contact.UpdatedAt.UTC(),
	}
}

// newContactResponses mapea una lista de terceros al DTO de salida. Devuelve un
// slice vacío (no nil) para que la API responda [] y no null.
func newContactResponses(contacts []model.Contact) []contactResponse {
	responses := make([]contactResponse, 0, len(contacts))
	for i := range contacts {
		responses = append(responses, newContactResponse(&contacts[i]))
	}
	return responses
}
