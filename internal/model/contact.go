package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Contact (tercero) es una persona o empresa con la que la panadería puede
// operar como cliente, proveedor, ambos o ninguno todavía. En esta etapa solo
// guarda datos básicos: los datos comerciales (pagos, cobros, pedidos) se
// agregarán más adelante.
//
// Al igual que User, es a la vez entidad de dominio y modelo GORM. Los campos
// opcionales se guardan como cadena vacía (nunca NULL) para simplificar las
// consultas y la serialización.
type Contact struct {
	ID           uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	Name         string         `gorm:"type:varchar(150);not null" json:"name"`
	BusinessName string         `gorm:"column:business_name;type:varchar(200)" json:"business_name,omitempty"`
	TaxID        string         `gorm:"column:tax_id;type:varchar(30)" json:"tax_id,omitempty"`
	Email        string         `gorm:"type:varchar(255)" json:"email,omitempty"`
	Phone        string         `gorm:"type:varchar(30)" json:"phone,omitempty"`
	WhatsApp     string         `gorm:"column:whatsapp;type:varchar(30)" json:"whatsapp,omitempty"`
	Address      string         `gorm:"type:varchar(255)" json:"address,omitempty"`
	City         string         `gorm:"type:varchar(100)" json:"city,omitempty"`
	Notes        string         `gorm:"type:text" json:"notes,omitempty"`
	IsClient     bool           `gorm:"column:is_client;not null" json:"is_client"`
	IsSupplier   bool           `gorm:"column:is_supplier;not null" json:"is_supplier"`
	IsActive     bool           `gorm:"column:is_active;not null" json:"is_active"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName fija el nombre de la tabla para no depender de la convención de GORM.
func (Contact) TableName() string {
	return "contacts"
}

// BeforeCreate genera el UUID del tercero antes de insertarlo.
//
// IsClient, IsSupplier e IsActive no llevan `default` en la etiqueta GORM a
// propósito: con un default, GORM omitiría un false explícito en el INSERT y la
// base aplicaría el valor por defecto. Los valores por defecto del dominio
// (is_active=true) los resuelve el service antes de persistir.
func (c *Contact) BeforeCreate(_ *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}
