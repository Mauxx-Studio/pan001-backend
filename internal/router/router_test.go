package router_test

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

	"github.com/pan001/backend/internal/handler"
	"github.com/pan001/backend/internal/infrastructure/auth"
	"github.com/pan001/backend/internal/model"
	"github.com/pan001/backend/internal/router"
	"github.com/pan001/backend/internal/service"
)

const testSecret = "secreto-de-pruebas"

// memUserRepository es un repositorio en memoria para montar el router completo
// sin depender de PostgreSQL.
type memUserRepository struct {
	byEmail map[string]*model.User
	byID    map[uuid.UUID]*model.User
}

func newMemUserRepository() *memUserRepository {
	return &memUserRepository{
		byEmail: map[string]*model.User{},
		byID:    map[uuid.UUID]*model.User{},
	}
}

func (r *memUserRepository) Create(_ context.Context, user *model.User) error {
	if _, exists := r.byEmail[user.Email]; exists {
		return model.ErrEmailAlreadyExists
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now().UTC()
	user.CreatedAt, user.UpdatedAt = now, now
	r.byEmail[user.Email] = user
	r.byID[user.ID] = user
	return nil
}

func (r *memUserRepository) FindByEmail(_ context.Context, email string) (*model.User, error) {
	user, ok := r.byEmail[email]
	if !ok || user.DeletedAt.Valid {
		return nil, model.ErrUserNotFound
	}
	return user, nil
}

func (r *memUserRepository) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	user, ok := r.byID[id]
	if !ok || user.DeletedAt.Valid {
		return nil, model.ErrUserNotFound
	}
	return user, nil
}

func (r *memUserRepository) UpdateLastLogin(_ context.Context, id uuid.UUID, loggedAt time.Time) error {
	user, ok := r.byID[id]
	if !ok {
		return model.ErrUserNotFound
	}
	user.LastLoginAt = &loggedAt
	return nil
}

// newTestApp monta las rutas reales con el middleware JWT real y un
// repositorio en memoria.
func newTestApp(t *testing.T) (*fiber.App, *memUserRepository) {
	t.Helper()

	repo := newMemUserRepository()
	tokens := auth.NewJWTManager(testSecret, auth.DefaultTokenTTL)
	authHandler := handler.NewAuthHandler(service.NewAuthService(repo, tokens))

	app := fiber.New()
	router.Register(app, router.Handlers{Auth: authHandler}, tokens)

	return app, repo
}

// do hace la petición y devuelve status + body.
func do(t *testing.T, app *fiber.App, method, path, body, token string) (int, string) {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	}
	if token != "" {
		req.Header.Set(fiber.HeaderAuthorization, "Bearer "+token)
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

// register crea un usuario y devuelve el token emitido en la respuesta.
func register(t *testing.T, app *fiber.App, email, role string) string {
	t.Helper()

	body := `{"email":"` + email + `","password":"secreto123","full_name":"María Pérez","role":"` + role + `"}`
	status, responseBody := do(t, app, http.MethodPost, "/api/v1/auth/register", body, "")
	if status != fiber.StatusCreated {
		t.Fatalf("register: status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, responseBody)
	}

	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		t.Fatalf("register: respuesta no es JSON válido: %v (body: %s)", err, responseBody)
	}
	if response.Token == "" {
		t.Fatal("register: la respuesta no incluye token")
	}

	return response.Token
}

func TestRegisterYLoginSonPublicos(t *testing.T) {
	app, _ := newTestApp(t)

	// Register sin Authorization: debe seguir siendo público.
	status, body := do(t, app, http.MethodPost, "/api/v1/auth/register",
		`{"email":"maria@pan001.com","password":"secreto123","full_name":"María Pérez","role":"cajero"}`, "")
	if status != fiber.StatusCreated {
		t.Fatalf("register: status = %d, se esperaba %d (body: %s)", status, fiber.StatusCreated, body)
	}

	// Login sin Authorization: debe seguir siendo público.
	status, body = do(t, app, http.MethodPost, "/api/v1/auth/login",
		`{"email":"maria@pan001.com","password":"secreto123"}`, "")
	if status != fiber.StatusOK {
		t.Fatalf("login: status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var response struct {
		Token string `json:"token"`
		User  struct {
			Email string `json:"email"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("login: respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if response.Token == "" || response.User.Email != "maria@pan001.com" {
		t.Errorf("login: respuesta inesperada: %+v", response)
	}
}

func TestMeSinTokenResponde401(t *testing.T) {
	app, _ := newTestApp(t)

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", "")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}

	var response struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if response.Error != "falta el header Authorization" {
		t.Errorf("error = %q, se esperaba el mensaje de header faltante", response.Error)
	}
}

func TestMeConTokenValidoDevuelveUsuario(t *testing.T) {
	app, _ := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleCajero)

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", token)

	if status != fiber.StatusOK {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusOK, body)
	}

	var response struct {
		ID       string `json:"id"`
		Email    string `json:"email"`
		FullName string `json:"full_name"`
		Role     string `json:"role"`
		IsActive bool   `json:"is_active"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}

	if response.ID == "" || response.ID == uuid.Nil.String() {
		t.Errorf("id = %q, se esperaba el UUID del usuario", response.ID)
	}
	if response.Email != "maria@pan001.com" {
		t.Errorf("email = %q, se esperaba maria@pan001.com", response.Email)
	}
	if response.FullName != "María Pérez" {
		t.Errorf("full_name = %q, se esperaba María Pérez", response.FullName)
	}
	if response.Role != model.RoleCajero {
		t.Errorf("role = %q, se esperaba %q", response.Role, model.RoleCajero)
	}
	if !response.IsActive {
		t.Error("is_active = false, el usuario recién registrado debe estar activo")
	}
	if strings.Contains(body, "password") {
		t.Errorf("la respuesta no debe exponer la contraseña: %s", body)
	}
}

func TestMeTokenInvalidoResponde401(t *testing.T) {
	app, _ := newTestApp(t)

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", "no-es-un-jwt")

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}

	var response struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("respuesta no es JSON válido: %v (body: %s)", err, body)
	}
	if response.Error != "token inválido o expirado" {
		t.Errorf("error = %q, se esperaba \"token inválido o expirado\"", response.Error)
	}
}

func TestMeUsuarioEliminadoResponde401(t *testing.T) {
	app, repo := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	// El token sigue vigente, pero el usuario ya no existe.
	delete(repo.byEmail, "maria@pan001.com")
	repo.byID = map[uuid.UUID]*model.User{}

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", token)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
	if !strings.Contains(body, "el usuario del token ya no existe") {
		t.Errorf("body = %q, se esperaba el mensaje de usuario inexistente", body)
	}
}

func TestMeUsuarioInactivoResponde401(t *testing.T) {
	app, repo := newTestApp(t)
	token := register(t, app, "maria@pan001.com", model.RoleVendedor)

	// Un admin desactiva la cuenta después de emitir el token.
	repo.byEmail["maria@pan001.com"].IsActive = false

	status, body := do(t, app, http.MethodGet, "/api/v1/auth/me", "", token)

	if status != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, se esperaba %d (body: %s)", status, fiber.StatusUnauthorized, body)
	}
	if !strings.Contains(body, "el usuario está desactivado") {
		t.Errorf("body = %q, se esperaba el mensaje de usuario desactivado", body)
	}
}
