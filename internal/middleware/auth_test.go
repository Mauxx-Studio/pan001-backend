package middleware_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/pan001/backend/internal/infrastructure/auth"
	"github.com/pan001/backend/internal/middleware"
	"github.com/pan001/backend/internal/model"
)

const (
	testSecret = "secreto-de-pruebas"
	testEmail  = "maria@pan001.com"
)

// testUser es el usuario cuyos claims viajan en los tokens de prueba.
func testUser() *model.User {
	return &model.User{
		ID:    uuid.MustParse("6f1b1f0e-3f8e-4a1c-9d0b-2b7a1c5e9f10"),
		Email: testEmail,
		Role:  model.RoleAdmin,
	}
}

// newTestApp monta una ruta pública y un grupo protegido con los middlewares
// reales, igual que hace router.Register.
func newTestApp() (*fiber.App, *auth.JWTManager) {
	tokens := auth.NewJWTManager(testSecret, auth.DefaultTokenTTL)

	app := fiber.New()
	app.Get("/public", func(c fiber.Ctx) error {
		return c.SendString("público")
	})

	protected := app.Group("", middleware.Authenticate(tokens))
	protected.Get("/me", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"user_id": middleware.GetUserID(c).String(),
			"email":   middleware.GetUserEmail(c),
			"role":    middleware.GetUserRole(c),
		})
	})
	protected.Get("/admin", middleware.RequireRole(model.RoleAdmin), func(c fiber.Ctx) error {
		return c.SendString("admin ok")
	})

	return app, tokens
}

// get hace la petición con el header Authorization indicado ("" = sin header).
func get(t *testing.T, app *fiber.App, path, authorization string) (int, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authorization != "" {
		req.Header.Set(fiber.HeaderAuthorization, authorization)
	}

	resp, err := app.Test(req, fiber.TestConfig{Timeout: 10 * time.Second})
	if err != nil {
		t.Fatalf("app.Test() devolvió un error: %v", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("no se pudo leer el body: %v", err)
	}

	return resp.StatusCode, string(raw)
}

// errorMessage extrae el campo "error" de una respuesta JSON.
func errorMessage(t *testing.T, body string) string {
	t.Helper()

	var response struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("la respuesta de error no es JSON válido: %v (body: %s)", err, body)
	}
	return response.Error
}

func TestAuthenticateTokenValidoGuardaIdentidad(t *testing.T) {
	app, tokens := newTestApp()
	user := testUser()

	token, err := tokens.Generate(user)
	if err != nil {
		t.Fatalf("Generate() devolvió un error inesperado: %v", err)
	}

	status, body := get(t, app, "/me", "Bearer "+token)
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var response struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}

	if response.UserID != user.ID.String() {
		t.Errorf("user_id = %q, se esperaba %q", response.UserID, user.ID)
	}
	if response.Email != user.Email {
		t.Errorf("email = %q, se esperaba %q", response.Email, user.Email)
	}
	if response.Role != user.Role {
		t.Errorf("role = %q, se esperaba %q", response.Role, user.Role)
	}
}

func TestAuthenticateSinHeaderResponde401(t *testing.T) {
	app, _ := newTestApp()

	status, body := get(t, app, "/me", "")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
	if msg := errorMessage(t, body); msg != "falta el header Authorization" {
		t.Errorf("error = %q, se esperaba el mensaje de header faltante", msg)
	}
}

func TestAuthenticateFormatoInvalidoResponde401(t *testing.T) {
	app, tokens := newTestApp()

	token, err := tokens.Generate(testUser())
	if err != nil {
		t.Fatalf("Generate() devolvió un error inesperado: %v", err)
	}

	casos := map[string]string{
		"sin esquema":      token,
		"otro esquema":     "Basic " + token,
		"sin credencial":   "Bearer",
		"credencial extra": "Bearer " + token + " extra",
	}

	for name, authorization := range casos {
		t.Run(name, func(t *testing.T) {
			status, body := get(t, app, "/me", authorization)

			if status != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
			}
			if msg := errorMessage(t, body); !strings.Contains(msg, "formato de Authorization inválido") {
				t.Errorf("error = %q, se esperaba un mensaje sobre el formato", msg)
			}
		})
	}
}

func TestAuthenticateTokenInvalidoResponde401(t *testing.T) {
	app, _ := newTestApp()

	// Token bien formado pero firmado con otro secreto.
	foreign, err := auth.NewJWTManager("otro-secreto", auth.DefaultTokenTTL).Generate(testUser())
	if err != nil {
		t.Fatalf("Generate() devolvió un error inesperado: %v", err)
	}

	casos := map[string]string{
		"no es un jwt":   "no-es-un-jwt",
		"firma extraña":  foreign,
		"token truncado": foreign[:len(foreign)-6],
	}

	for name, token := range casos {
		t.Run(name, func(t *testing.T) {
			status, body := get(t, app, "/me", "Bearer "+token)

			if status != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
			}
			if msg := errorMessage(t, body); msg != "token inválido o expirado" {
				t.Errorf("error = %q, se esperaba \"token inválido o expirado\"", msg)
			}
		})
	}
}

func TestAuthenticateTokenExpiradoResponde401(t *testing.T) {
	app, _ := newTestApp()

	// Se firma con el secreto correcto pero con exp en el pasado.
	user := testUser()
	claims := auth.Claims{
		UserID: user.ID.String(),
		Email:  user.Email,
		Role:   user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.String(),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("no se pudo firmar el token de prueba: %v", err)
	}

	status, body := get(t, app, "/me", "Bearer "+token)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
	if msg := errorMessage(t, body); msg != "token inválido o expirado" {
		t.Errorf("error = %q, se esperaba \"token inválido o expirado\"", msg)
	}
}

func TestAuthenticateTokenConUserIDSinUUIDResponde401(t *testing.T) {
	app, _ := newTestApp()

	// Firma correcta, pero user_id no es un UUID: el token es inválido.
	claims := auth.Claims{
		UserID: "no-es-un-uuid",
		Email:  testEmail,
		Role:   model.RoleAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("no se pudo firmar el token de prueba: %v", err)
	}

	status, body := get(t, app, "/me", "Bearer "+token)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
	if msg := errorMessage(t, body); msg != "token inválido o expirado" {
		t.Errorf("error = %q, se esperaba \"token inválido o expirado\"", msg)
	}
}

func TestRutaPublicaNoRequiereToken(t *testing.T) {
	app, _ := newTestApp()

	status, body := get(t, app, "/public", "")

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}
	if body != "público" {
		t.Errorf("body = %q, se esperaba \"público\"", body)
	}
}

func TestHelpersSinAutenticacionDevuelvenVacios(t *testing.T) {
	app := fiber.New()
	app.Get("/helpers", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"user_id": middleware.GetUserID(c).String(),
			"email":   middleware.GetUserEmail(c),
			"role":    middleware.GetUserRole(c),
		})
	})

	status, body := get(t, app, "/helpers", "")
	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var response struct {
		UserID string `json:"user_id"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}

	if response.UserID != uuid.Nil.String() {
		t.Errorf("user_id = %q, se esperaba uuid.Nil", response.UserID)
	}
	if response.Email != "" || response.Role != "" {
		t.Errorf("email/role = %q/%q, se esperaban vacíos sin autenticación", response.Email, response.Role)
	}
}

func TestRequireRole(t *testing.T) {
	app, tokens := newTestApp()

	adminToken, err := tokens.Generate(&model.User{
		ID:    uuid.New(),
		Email: "admin@pan001.com",
		Role:  model.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("Generate() devolvió un error inesperado: %v", err)
	}
	cajeroToken, err := tokens.Generate(&model.User{
		ID:    uuid.New(),
		Email: "caja@pan001.com",
		Role:  model.RoleCajero,
	})
	if err != nil {
		t.Fatalf("Generate() devolvió un error inesperado: %v", err)
	}

	t.Run("rol permitido", func(t *testing.T) {
		status, body := get(t, app, "/admin", "Bearer "+adminToken)
		if status != fiber.StatusOK {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
		}
	})

	t.Run("rol insuficiente", func(t *testing.T) {
		status, body := get(t, app, "/admin", "Bearer "+cajeroToken)
		if status != fiber.StatusForbidden {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusForbidden, body)
		}
		if msg := errorMessage(t, body); !strings.Contains(msg, "se requiere rol admin") {
			t.Errorf("error = %q, se esperaba que mencione el rol requerido", msg)
		}
	})

	t.Run("sin token", func(t *testing.T) {
		status, body := get(t, app, "/admin", "")
		if status != fiber.StatusUnauthorized {
			t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
		}
	})
}
