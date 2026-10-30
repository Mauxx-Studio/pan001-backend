package database

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/pan001/backend/internal/model"
)

// AutoMigrate crea o actualiza las tablas de los modelos del dominio al
// arrancar la aplicación. Es idempotente: si la tabla existe, solo agrega las
// columnas, índices y constraints que falten (no borra datos).
//
// A medida que se agreguen modelos, se suman acá:
//
//	db.AutoMigrate(&model.User{}, &model.Product{}, ...)
//
// Para producción conviene reemplazarlo por migraciones versionadas en
// migrations/ ejecutadas desde un cmd/migrate.
func AutoMigrate(db *gorm.DB) error {
	if err := db.AutoMigrate(&model.User{}); err != nil {
		return fmt.Errorf("aplicando AutoMigrate: %w", err)
	}
	return nil
}
