// Package handler expone la API HTTP. Traduce peticiones a casos de uso y
// errores de dominio a códigos de estado: no contiene reglas de negocio.
package handler

import (
	"errors"
	"log"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v3"

	"github.com/pan001/backend/internal/middleware"
	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/service"
)

// AuthHandler agrupa los endpoints de registro y login.
type AuthHandler struct {
	auth     *service.AuthService
	validate *validator.Validate
}

// NewAuthHandler construye el handler de autenticación.
func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{
		auth:     auth,
		validate: newValidator(),
	}
}

// Register atiende POST /api/v1/auth/register.
//
// Por ahora está abierto para desarrollo; más adelante se restringirá a admin.
func (h *AuthHandler) Register(c fiber.Ctx) error {
	var req registerRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el body debe ser un JSON válido")
	}
	if err := h.validate.Struct(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationMessage(err))
	}

	result, err := h.auth.Register(c.Context(), service.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
		FullName: req.FullName,
		Phone:    req.Phone,
		Role:     req.Role,
	})
	if err != nil {
		return authError("register", err)
	}

	return c.Status(fiber.StatusCreated).JSON(newAuthResponse(result))
}

// Login atiende POST /api/v1/auth/login.
func (h *AuthHandler) Login(c fiber.Ctx) error {
	var req loginRequest
	if err := c.Bind().JSON(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el body debe ser un JSON válido")
	}
	if err := h.validate.Struct(req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, validationMessage(err))
	}

	result, err := h.auth.Login(c.Context(), req.Email, req.Password)
	if err != nil {
		return authError("login", err)
	}

	return c.Status(fiber.StatusOK).JSON(newAuthResponse(result))
}

// Me atiende GET /api/v1/auth/me, una ruta protegida: el middleware de
// autenticación ya validó el token y dejó el user_id en el contexto.
func (h *AuthHandler) Me(c fiber.Ctx) error {
	user, err := h.auth.Profile(c.Context(), middleware.GetUserID(c))
	if err != nil {
		return profileError(err)
	}

	return c.Status(fiber.StatusOK).JSON(newUserResponse(user))
}

// profileError traduce los errores de GET /me. Un usuario borrado o
// desactivado después de emitir el token ya no puede usar la API.
func profileError(err error) error {
	switch {
	case errors.Is(err, model.ErrUserNotFound):
		return fiber.NewError(fiber.StatusUnauthorized, "el usuario del token ya no existe")
	case errors.Is(err, model.ErrUserInactive):
		return fiber.NewError(fiber.StatusUnauthorized, "el usuario está desactivado")
	default:
		log.Printf("error en me: %v", err)
		return err
	}
}

// authError traduce los errores de dominio a errores HTTP con mensajes claros.
// Cualquier error no previsto se loguea y se devuelve tal cual para que el
// errorHandler global responda 500 sin filtrar detalles internos.
func authError(operation string, err error) error {
	switch {
	case errors.Is(err, model.ErrEmailAlreadyExists):
		return fiber.NewError(fiber.StatusConflict, "el email ya está registrado")
	case errors.Is(err, model.ErrInvalidCredentials):
		return fiber.NewError(fiber.StatusUnauthorized, "email o contraseña incorrectos")
	case errors.Is(err, model.ErrUserNotFound):
		// Mismo mensaje que credenciales inválidas para no revelar si el email existe.
		return fiber.NewError(fiber.StatusUnauthorized, "email o contraseña incorrectos")
	case errors.Is(err, model.ErrUserInactive):
		return fiber.NewError(fiber.StatusForbidden, "el usuario está desactivado")
	case errors.Is(err, model.ErrInvalidRole), errors.Is(err, model.ErrPasswordTooLong):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		log.Printf("error en %s: %v", operation, err)
		return err
	}
}
