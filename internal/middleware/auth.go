// Package middleware contiene los middlewares HTTP transversales de la API
// (autenticación y autorización por rol).
//
// Los middlewares son adaptadores: no contienen reglas de negocio, solo
// traducen el header Authorization a una identidad disponible en el contexto de
// Fiber para el resto de la cadena de handlers.
package middleware

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/pan001/backend/internal/infrastructure/auth"
)

const (
	// bearerScheme es el esquema exigido en el header Authorization.
	bearerScheme = "Bearer"

	// Mensajes de error de autenticación. Se centralizan para que las
	// respuestas sean claras y consistentes.
	msgMissingHeader = "falta el header Authorization"
	msgInvalidFormat = "formato de Authorization inválido: se esperaba 'Bearer <token>'"
	msgInvalidToken  = "token inválido o expirado"
	msgNoIdentity    = "no hay un usuario autenticado en el contexto"
)

// Errores internos de parseo del header. Se usan solo para distinguir el motivo
// del 401; el mensaje que ve el cliente es el texto del error.
var (
	errMissingToken  = errors.New(msgMissingHeader)
	errInvalidFormat = errors.New(msgInvalidFormat)
)

// TokenParser es el puerto de validación de access tokens. La implementación
// concreta es auth.JWTManager, el mismo gestor que firma los tokens en login y
// register, así que la firma y el secreto (JWT_SECRET) no se duplican.
type TokenParser interface {
	Parse(token string) (*auth.Claims, error)
}

// Authenticate valida el access token del header Authorization y guarda la
// identidad del usuario en el contexto de Fiber antes de llamar a c.Next().
//
// Responde 401 con { "error": "..." } cuando:
//   - falta el header Authorization;
//   - el formato no es "Bearer <token>";
//   - el token está expirado, la firma es inválida o los claims no son válidos.
//
// Se puede aplicar a un grupo completo de rutas para no repetirlo en cada una:
//
//	protected := api.Group("", middleware.Authenticate(tokens))
//	protected.Get("/auth/me", handlers.Auth.Me)
func Authenticate(tokens TokenParser) fiber.Handler {
	return func(c fiber.Ctx) error {
		token, err := bearerToken(c.Get(fiber.HeaderAuthorization))
		if err != nil {
			return unauthorized(c, err.Error())
		}

		claims, err := tokens.Parse(token)
		if err != nil {
			// No se expone el detalle (firma, expiración, algoritmo): un
			// atacante no debe saber por qué falló la validación.
			return unauthorized(c, msgInvalidToken)
		}

		userID, err := uuid.Parse(claims.UserID)
		if err != nil {
			// Los tokens que emite esta API siempre llevan un UUID como
			// user_id; cualquier otra cosa es un token inválido.
			return unauthorized(c, msgInvalidToken)
		}

		c.Locals(LocalsUserID, userID)
		c.Locals(LocalsEmail, claims.Email)
		c.Locals(LocalsRole, claims.Role)

		return c.Next()
	}
}

// bearerToken extrae el token del header Authorization, que debe tener el
// formato "Bearer <token>". El nombre del esquema no distingue mayúsculas
// (RFC 7235), pero la credencial no puede faltar.
func bearerToken(header string) (string, error) {
	if strings.TrimSpace(header) == "" {
		return "", errMissingToken
	}

	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], bearerScheme) {
		return "", errInvalidFormat
	}

	return parts[1], nil
}

// unauthorized responde 401 con el mismo formato de error que el errorHandler
// global de main ({"error": "..."}) directamente desde el middleware.
func unauthorized(c fiber.Ctx, message string) error {
	return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": message})
}
