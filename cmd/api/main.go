// Command api es el punto de entrada del backend REST de pan001.
//
// Uso:
//
//	go run ./cmd/api
//	PORT=9090 go run ./cmd/api
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"gorm.io/gorm"

	"github.com/pan001/backend/internal/config"
	"github.com/pan001/backend/internal/handler"
	"github.com/pan001/backend/internal/infrastructure/auth"
	"github.com/pan001/backend/internal/infrastructure/database"
	"github.com/pan001/backend/internal/router"
	"github.com/pan001/backend/internal/service"
)

const (
	// shutdownTimeout es el tiempo máximo de espera para cerrar las conexiones en curso.
	shutdownTimeout = 10 * time.Second
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("configuración inválida: %v", err)
	}

	// La base de datos se abre antes de levantar el servidor: sin ella la API no
	// puede atender peticiones, así que si falla terminamos el proceso.
	db, err := database.New(cfg.Database, cfg.App.IsDevelopment())
	if err != nil {
		log.Fatalf("no se pudo conectar a la base de datos: %v", err)
	}
	log.Printf("conectado a PostgreSQL en %s", cfg.Database) // Database.String() omite la contraseña.

	// Cerramos el pool de conexiones al salir para no dejar sesiones abiertas.
	defer closeDatabase(db)

	// AutoMigrate crea o actualiza las tablas del dominio antes de aceptar tráfico.
	if err := database.AutoMigrate(db); err != nil {
		closeDatabase(db)
		log.Fatalf("no se pudieron aplicar las migraciones: %v", err)
	}
	log.Println("migraciones aplicadas (AutoMigrate: users)")

	if cfg.Auth.UsingDefaultSecret() {
		log.Println("advertencia: usando el secreto JWT de desarrollo; definí JWT_SECRET para cambiarlo")
	}

	// Wiring de dependencias: repositorio → caso de uso → handler → router.
	// tokens firma los access tokens (register/login) y valida los del
	// middleware de autenticación: ambos usan el mismo secreto (JWT_SECRET).
	userRepo := database.NewUserRepository(db)
	tokens := auth.NewJWTManager(cfg.Auth.JWTSecret, auth.DefaultTokenTTL)
	authService := service.NewAuthService(userRepo, tokens)
	authHandler := handler.NewAuthHandler(authService)

	app := newApp(db)
	router.Register(app, router.Handlers{Auth: authHandler}, tokens)

	// Listen bloquea, así que arrancamos el servidor en una goroutine para poder
	// capturar las señales del sistema operativo y apagarlo de forma ordenada.
	errCh := make(chan error, 1)
	go func() {
		// En Fiber v3 las opciones de arranque (banner, prefork, TLS...) van en
		// ListenConfig, no en fiber.Config.
		err := app.Listen(":"+cfg.App.Port, fiber.ListenConfig{
			DisableStartupMessage: true, // el arranque lo logueamos nosotros con la URL completa
		})
		if err != nil {
			errCh <- err
		}
	}()
	log.Printf("API escuchando en http://localhost:%s", cfg.App.Port)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		// log.Fatalf termina el proceso sin ejecutar los defer, así que cerramos
		// el pool de conexiones a mano antes de salir.
		closeDatabase(db)
		log.Fatalf("no se pudo iniciar el servidor: %v", err)
	case <-ctx.Done():
		log.Println("apagando el servidor...")
	}

	// Dejamos de aceptar conexiones nuevas y damos margen a las peticiones en curso.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Printf("error durante el apagado: %v", err)
		return
	}
	log.Println("servidor detenido correctamente")
}

// newApp construye la aplicación Fiber con su configuración y las rutas que no
// dependen de la base de datos.
//
// La conexión se recibe para el health check; los grupos de rutas del dominio
// se montan en main con router.Register una vez armado el wiring.
func newApp(db *gorm.DB) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName:      "pan001 backend",
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		ErrorHandler: errorHandler,
	})

	app.Get("/", hello)
	app.Get("/health", health(db))

	return app
}

// hello responde con un JSON simple para verificar que el servicio está vivo.
// En Fiber v3 los handlers reciben fiber.Ctx como valor, porque Ctx es una interfaz.
func hello(c fiber.Ctx) error {
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"message": "Hola mundo con Fiber"})
}

// health comprueba que la API esté levantada y que la conexión con PostgreSQL
// siga viva (hace un ping real contra la base de datos).
func health(db *gorm.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		sqlDB, err := db.DB()
		if err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "base de datos no disponible")
		}

		if err := sqlDB.PingContext(c.Context()); err != nil {
			return fiber.NewError(fiber.StatusServiceUnavailable, "base de datos no disponible")
		}

		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"status":   "ok",
			"database": "up",
		})
	}
}

// closeDatabase cierra el pool de conexiones logueando cualquier error.
func closeDatabase(db *gorm.DB) {
	if err := database.Close(db); err != nil {
		log.Printf("error al cerrar la base de datos: %v", err)
	}
}

// errorHandler unifica el formato de todos los errores HTTP en JSON.
func errorHandler(c fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "error interno del servidor"

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		code = fiberErr.Code
		message = fiberErr.Message
	}

	return c.Status(code).JSON(fiber.Map{"error": message})
}
