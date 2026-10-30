// Package database centraliza la creación de conexiones a los almacenes de
// datos del proyecto. Hoy expone la conexión a PostgreSQL a través de GORM.
package database

import (
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/pan001/backend/internal/config"
)

const (
	// maxOpenConns limita las conexiones abiertas simultáneamente contra PostgreSQL.
	maxOpenConns = 25
	// maxIdleConns mantiene un mínimo de conexiones listas para reutilizar.
	maxIdleConns = 5
	// connMaxLifetime recicla las conexiones para no quedarnos con sesiones viejas.
	connMaxLifetime = time.Hour
	// connMaxIdleTime cierra las conexiones ociosas.
	connMaxIdleTime = 10 * time.Minute
	// slowQueryThreshold es el tiempo a partir del cual una consulta se loguea como lenta.
	slowQueryThreshold = 200 * time.Millisecond
)

// New abre la conexión con PostgreSQL usando GORM, configura el pool de
// conexiones y verifica con un ping que el servidor responde.
//
// Devuelve el *gorm.DB listo para inyectarse en los repositorios; cualquier
// fallo (DSN inválido, servidor caído, credenciales incorrectas, base
// inexistente) se devuelve envuelto para conservar su causa original.
//
// El parámetro debug activa el logger detallado de GORM (todas las consultas
// SQL), pensado para desarrollo local.
func New(cfg config.Database, debug bool) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: newLogger(debug),
		// TranslateError traduce los errores del driver a los errores de GORM
		// (gorm.ErrDuplicatedKey, gorm.ErrForeignKeyViolated, ...).
		TranslateError: true,
		// Guardamos las fechas en UTC para no depender de la zona horaria del servidor.
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("conectando con PostgreSQL (%s): %w", cfg, err)
	}

	// database/sql mantiene el pool real de conexiones que usa GORM.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obteniendo el pool de conexiones de PostgreSQL (%s): %w", cfg, err)
	}

	sqlDB.SetMaxOpenConns(maxOpenConns)
	sqlDB.SetMaxIdleConns(maxIdleConns)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)
	sqlDB.SetConnMaxIdleTime(connMaxIdleTime)

	// Verificamos la conexión al arrancar para fallar rápido y con un error claro.
	if err := sqlDB.Ping(); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("verificando la conexión con PostgreSQL (%s): %w", cfg, err)
	}

	return db, nil
}

// Close libera el pool de conexiones asociado a db. Se llama al apagar la
// aplicación para no dejar sesiones abiertas en PostgreSQL.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("obteniendo el pool de conexiones de PostgreSQL: %w", err)
	}

	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("cerrando la conexión con PostgreSQL: %w", err)
	}

	return nil
}

// newLogger devuelve el logger de GORM: en desarrollo muestra todas las
// consultas SQL con colores y marca las lentas; fuera de desarrollo solo
// registra warnings y errores para no ensuciar los logs.
func newLogger(debug bool) logger.Interface {
	logConfig := logger.Config{
		SlowThreshold:             slowQueryThreshold,
		IgnoreRecordNotFoundError: true,
	}

	if debug {
		logConfig.LogLevel = logger.Info
		logConfig.Colorful = true
		return logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logConfig)
	}

	logConfig.LogLevel = logger.Warn
	return logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logConfig)
}

// Migraciones
//
// Las tablas se crean con database.AutoMigrate (ver migrate.go), que se ejecuta
// al arrancar la aplicación. La carpeta migrations/ queda reservada para las
// migraciones versionadas de producción.
