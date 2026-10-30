package handler_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/pan001/backend/internal/handler"
	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/service"
)

// memUserRepository es un repositorio en memoria para no depender de PostgreSQL.
type memUserRepository struct {
	users map[string]*model.User
}

func newMemUserRepository() *memUserRepository {
	return &memUserRepository{users: map[string]*model.User{}}
}

func (r *memUserRepository) Create(_ context.Context, user *model.User) error {
	if _, exists := r.users[user.Email]; exists {
		return model.ErrEmailAlreadyExists
	}
	user.ID = uuid.New()
	user.CreatedAt, user.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	r.users[user.Email] = user
	return nil
}

func (r *memUserRepository) FindByEmail(_ context.Context, email string) (*model.User, error) {
	user, ok := r.users[email]
	if !ok || user.DeletedAt.Valid {
		return nil, model.ErrUserNotFound
	}
	return user, nil
}

func (r *memUserRepository) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	for _, user := range r.users {
		if user.ID == id {
			return user, nil
		}
	}
	return nil, model.ErrUserNotFound
}

func (r *memUserRepository) UpdateLastLogin(_ context.Context, id uuid.UUID, loggedAt time.Time) error {
	for _, user := range r.users {
		if user.ID == id {
			user.LastLoginAt = &loggedAt
			return nil
		}
	}
	return model.ErrUserNotFound
}

// staticTokenGenerator devuelve siempre el mismo token.
type staticTokenGenerator struct{}

func (staticTokenGenerator) Generate(*model.User) (string, error) { return "token-de-prueba", nil }

// newTestApp monta un Fiber mínimo con las mismas rutas que usa router.Register.
// El error handler por defecto de Fiber respeta el código de fiber.NewError, así
// que los tests validan los status reales sin duplicar el errorHandler de main.
func newTestApp(repo *memUserRepository) *fiber.App {
	svc := service.NewAuthService(repo, staticTokenGenerator{})
	authHandler := handler.NewAuthHandler(svc)

	app := fiber.New()
	app.Post("/auth/register", authHandler.Register)
	app.Post("/auth/login", authHandler.Login)
	return app
}

// do hace la petición y devuelve status + body.
func do(t *testing.T, app *fiber.App, path, body string) (int, string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

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

// testUser crea un usuario con la contraseña "secreto123" ya hasheada.
func testUser(t *testing.T, email string, active bool) *model.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("secreto123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("no se pudo hashear la contraseña: %v", err)
	}

	return &model.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		FullName:     "Usuario de prueba",
		Role:         model.RoleVendedor,
		IsActive:     active,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
}

func TestRegisterResponde201SinPassword(t *testing.T) {
	app := newTestApp(newMemUserRepository())

	status, body := do(t, app, "/auth/register",
		`{"email":"maria@pan001.com","password":"secreto123","full_name":"Maria Perez","phone":"1155550000","role":"cajero"}`)

	if status != fiber.StatusCreated {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}

	var response struct {
		Token string `json:"token"`
		User  struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if response.Token != "token-de-prueba" {
		t.Errorf("token = %q, se esperaba el token del generador", response.Token)
	}
	if response.User.Email != "maria@pan001.com" || response.User.Role != "cajero" {
		t.Errorf("user = %+v, valores inesperados", response.User)
	}
	if strings.Contains(body, "password") || strings.Contains(body, "secreto123") {
		t.Errorf("la respuesta no debe exponer la contraseña: %s", body)
	}
}

func TestRegisterEmailDuplicadoResponde409(t *testing.T) {
	repo := newMemUserRepository()
	repo.users["maria@pan001.com"] = testUser(t, "maria@pan001.com", true)
	app := newTestApp(repo)

	status, body := do(t, app, "/auth/register",
		`{"email":"maria@pan001.com","password":"secreto123","full_name":"Maria Perez"}`)

	if status != fiber.StatusConflict {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusConflict, body)
	}
}

func TestRegisterBodyInvalidoResponde400(t *testing.T) {
	app := newTestApp(newMemUserRepository())

	casos := map[string]string{
		"email inválido":  `{"email":"no-es-email","password":"secreto123","full_name":"Maria Perez"}`,
		"password corta":  `{"email":"maria@pan001.com","password":"123","full_name":"Maria Perez"}`,
		"sin full_name":   `{"email":"maria@pan001.com","password":"secreto123"}`,
		"rol inválido":    `{"email":"maria@pan001.com","password":"secreto123","full_name":"Maria Perez","role":"root"}`,
		"json malformado": `{"email":`,
	}

	for name, body := range casos {
		t.Run(name, func(t *testing.T) {
			status, respBody := do(t, app, "/auth/register", body)
			if status != fiber.StatusBadRequest {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusBadRequest, respBody)
			}
		})
	}
}

func TestLoginResponde200IncluyeTokenYLastLogin(t *testing.T) {
	repo := newMemUserRepository()
	repo.users["maria@pan001.com"] = testUser(t, "maria@pan001.com", true)
	app := newTestApp(repo)

	status, body := do(t, app, "/auth/login",
		`{"email":"MARIA@pan001.com","password":"secreto123"}`)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var response struct {
		Token string `json:"token"`
		User  struct {
			LastLoginAt *time.Time `json:"last_login_at"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if response.Token == "" {
		t.Error("token vacío, se esperaba un access token")
	}
	if response.User.LastLoginAt == nil {
		t.Error("last_login_at = nil, se esperaba la fecha del login")
	}
	if strings.Contains(body, "password") {
		t.Errorf("la respuesta no debe exponer la contraseña: %s", body)
	}
}

func TestLoginCredencialesInvalidasResponde401(t *testing.T) {
	repo := newMemUserRepository()
	repo.users["maria@pan001.com"] = testUser(t, "maria@pan001.com", true)
	app := newTestApp(repo)

	casos := map[string]string{
		"password incorrecta": `{"email":"maria@pan001.com","password":"incorrecta"}`,
		"email inexistente":   `{"email":"nadie@pan001.com","password":"secreto123"}`,
	}

	for name, body := range casos {
		t.Run(name, func(t *testing.T) {
			status, respBody := do(t, app, "/auth/login", body)
			if status != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, respBody)
			}
		})
	}
}

func TestLoginUsuarioInactivoResponde403(t *testing.T) {
	repo := newMemUserRepository()
	repo.users["maria@pan001.com"] = testUser(t, "maria@pan001.com", false)
	app := newTestApp(repo)

	status, body := do(t, app, "/auth/login",
		`{"email":"maria@pan001.com","password":"secreto123"}`)

	if status != fiber.StatusForbidden {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusForbidden, body)
	}
}
