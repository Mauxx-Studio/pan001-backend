package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// RequireRole autoriza la petición solo si el usuario autenticado tiene uno de
// los roles indicados. Debe montarse siempre después de Authenticate, porque
// depende de la identidad que ese middleware deja en el contexto:
//
//	protected := api.Group("", middleware.Authenticate(tokens))
//	protected.Delete("/users/:id", middleware.RequireRole(model.RoleAdmin), handlers.Users.Delete)
//
// Si no hay identidad responde 401 (falta autenticarse); si el rol no alcanza,
// 403 (autenticado pero sin permiso).
func RequireRole(roles ...string) fiber.Handler {
	denied := "acceso denegado: rol insuficiente"
	if len(roles) > 0 {
		denied = "acceso denegado: se requiere rol " + strings.Join(roles, " o ")
	}

	return func(c fiber.Ctx) error {
		role := GetUserRole(c)
		if role == "" {
			return unauthorized(c, msgNoIdentity)
		}

		for _, allowed := range roles {
			if role == allowed {
				return c.Next()
			}
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"error": denied})
	}
}
