// Package router concentra el registro de rutas de la API para que cmd/api no
// conozca la forma exacta de cada grupo.
package router

import (
	"github.com/gofiber/fiber/v3"

	"github.com/pan001/backend/internal/handler"
	"github.com/pan001/backend/internal/middleware"
)

// Handlers agrupa los handlers que se montan en el router.
type Handlers struct {
	Auth     *handler.AuthHandler
	Contacts *handler.ContactHandler
}

// Register monta todos los grupos de rutas versionadas.
//
// Rutas públicas (no requieren token):
//
//	POST /api/v1/auth/register
//	POST /api/v1/auth/login
//
// Rutas protegidas (requieren "Authorization: Bearer <token>"):
//
//	GET    /api/v1/auth/me
//	POST   /api/v1/contacts
//	GET    /api/v1/contacts
//	GET    /api/v1/contacts/:id
//	PUT    /api/v1/contacts/:id
//	DELETE /api/v1/contacts/:id
//
// tokens valida los access tokens; debe ser el mismo gestor que los firma en
// login/register (auth.JWTManager) para compartir secreto y algoritmo.
func Register(app *fiber.App, handlers Handlers, tokens middleware.TokenParser) {
	api := app.Group("/api/v1")

	// Rutas públicas de autenticación: se registran antes del grupo protegido
	// para que el middleware JWT nunca las alcance.
	auth := api.Group("/auth")
	auth.Post("/register", handlers.Auth.Register)
	auth.Post("/login", handlers.Auth.Login)

	// Grupo protegido: el middleware se aplica a todo lo que se registre en él.
	protected := api.Group("", middleware.Authenticate(tokens))
	protected.Get("/auth/me", handlers.Auth.Me)

	// Terceros (contactos): CRUD completo detrás del JWT.
	contacts := protected.Group("/contacts")
	contacts.Post("", handlers.Contacts.Create)
	contacts.Get("", handlers.Contacts.List)
	contacts.Get("/:id", handlers.Contacts.GetByID)
	contacts.Put("/:id", handlers.Contacts.Update)
	contacts.Delete("/:id", handlers.Contacts.Delete)
}
