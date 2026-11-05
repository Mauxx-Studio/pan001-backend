package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// Product es un ítem del catálogo de la panadería: un producto terminado, un
// insumo o un empaque. Los tres casos comparten una sola tabla porque manejan
// los mismos campos; los flags describen para qué se usa cada ítem:
//
//   - IsSold: se vende.
//   - IsPurchased: se compra a proveedores.
//   - IsProduced: se elabora en la panadería (las recetas llegan más adelante).
//
// Los flags se combinan libremente (un ítem puede venderse y comprarse a la
// vez) y ninguno es obligatorio: el catálogo admite ítems solo informativos.
//
// Al igual que Contact, es entidad de dominio y modelo GORM a la vez. Las
// cantidades y los importes usan shopspring/decimal con columnas NUMERIC, de
// modo que nunca pasan por float32/float64 ni acumulan errores de redondeo.
type Product struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Name        string    `gorm:"type:varchar(150);not null" json:"name"`
	SKU         string    `gorm:"column:sku;type:varchar(50)" json:"sku,omitempty"`
	Description string    `gorm:"type:text" json:"description,omitempty"`
	Unit        string    `gorm:"type:varchar(20);not null" json:"unit"`

	IsSold      bool `gorm:"column:is_sold;not null" json:"is_sold"`
	IsPurchased bool `gorm:"column:is_purchased;not null" json:"is_purchased"`
	IsProduced  bool `gorm:"column:is_produced;not null" json:"is_produced"`
	TrackStock  bool `gorm:"column:track_stock;not null" json:"track_stock"`

	// numeric(14,3) admite hasta 99999999999.999 unidades y 3 decimales, que es
	// más que suficiente para kilos, gramos o litros.
	CurrentStock decimal.Decimal `gorm:"column:current_stock;type:numeric(14,3);not null" json:"current_stock"`
	MinStock     decimal.Decimal `gorm:"column:min_stock;type:numeric(14,3);not null" json:"min_stock"`
	// numeric(14,2) guarda importes con dos decimales (centavos).
	SalePrice decimal.Decimal `gorm:"column:sale_price;type:numeric(14,2);not null" json:"sale_price"`
	Cost      decimal.Decimal `gorm:"column:cost;type:numeric(14,2);not null" json:"cost"`

	IsActive  bool           `gorm:"column:is_active;not null" json:"is_active"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName fija el nombre de la tabla para no depender de la convención de GORM.
func (Product) TableName() string {
	return "products"
}

// BeforeCreate genera el UUID del producto antes de insertarlo.
//
// Igual que en Contact, los booleanos no llevan `default` en la etiqueta GORM a
// propósito: con un default, GORM omitiría un false explícito en el INSERT y la
// base aplicaría el valor por defecto. Los valores por defecto del dominio
// (track_stock=true, is_active=true, el resto false) los resuelve el service
// antes de persistir.
func (p *Product) BeforeCreate(_ *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}
