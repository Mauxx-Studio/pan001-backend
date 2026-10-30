package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
)

// Claves de c.Locals donde Authenticate guarda la identidad del usuario. Se
// exportan para poder leerlas desde otros middlewares sin repetir strings.
const (
	// LocalsUserID guarda el user_id del token ya convertido a uuid.UUID.
	LocalsUserID = "user_id"
	// LocalsEmail guarda el email del token.
	LocalsEmail = "email"
	// LocalsRole guarda el rol del token.
	LocalsRole = "role"
)

// GetUserID devuelve el ID del usuario autenticado. Si la petición no pasó por
// Authenticate, devuelve uuid.Nil.
func GetUserID(c fiber.Ctx) uuid.UUID {
	userID, ok := c.Locals(LocalsUserID).(uuid.UUID)
	if !ok {
		return uuid.Nil
	}
	return userID
}

// GetUserEmail devuelve el email del usuario autenticado, o "" si la petición
// no pasó por Authenticate.
func GetUserEmail(c fiber.Ctx) string {
	email, _ := c.Locals(LocalsEmail).(string)
	return email
}

// GetUserRole devuelve el rol del usuario autenticado, o "" si la petición no
// pasó por Authenticate.
func GetUserRole(c fiber.Ctx) string {
	role, _ := c.Locals(LocalsRole).(string)
	return role
}
