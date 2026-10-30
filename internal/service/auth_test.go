package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/pan001/backend/internal/model"
)

// fakeUserRepository es un repositorio en memoria que permite probar los casos
// de uso sin base de datos.
type fakeUserRepository struct {
	users map[string]*model.User // indexado por email normalizado
	byID  map[uuid.UUID]*model.User
}

func newFakeUserRepository() *fakeUserRepository {
	return &fakeUserRepository{
		users: map[string]*model.User{},
		byID:  map[uuid.UUID]*model.User{},
	}
}

func (r *fakeUserRepository) Create(_ context.Context, user *model.User) error {
	if _, exists := r.users[user.Email]; exists {
		return model.ErrEmailAlreadyExists
	}
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	now := time.Now().UTC()
	user.CreatedAt, user.UpdatedAt = now, now
	r.users[user.Email] = user
	r.byID[user.ID] = user
	return nil
}

func (r *fakeUserRepository) FindByEmail(_ context.Context, email string) (*model.User, error) {
	user, ok := r.users[email]
	if !ok || user.DeletedAt.Valid {
		return nil, model.ErrUserNotFound
	}
	return user, nil
}

func (r *fakeUserRepository) FindByID(_ context.Context, id uuid.UUID) (*model.User, error) {
	user, ok := r.byID[id]
	if !ok || user.DeletedAt.Valid {
		return nil, model.ErrUserNotFound
	}
	return user, nil
}

func (r *fakeUserRepository) UpdateLastLogin(_ context.Context, id uuid.UUID, loggedAt time.Time) error {
	user, ok := r.byID[id]
	if !ok {
		return model.ErrUserNotFound
	}
	user.LastLoginAt = &loggedAt
	return nil
}

// fakeTokenGenerator devuelve un token fijo y registra los usuarios para los
// que se pidió un token.
type fakeTokenGenerator struct {
	token string
	err   error
	calls []*model.User
}

func (g *fakeTokenGenerator) Generate(user *model.User) (string, error) {
	g.calls = append(g.calls, user)
	if g.err != nil {
		return "", g.err
	}
	return g.token, nil
}

// newTestUser crea un usuario con la contraseña indicada ya hasheada.
func newTestUser(t *testing.T, password string) *model.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("no se pudo hashear la contraseña de prueba: %v", err)
	}

	return &model.User{
		ID:           uuid.New(),
		Email:        "user@example.com",
		PasswordHash: string(hash),
		FullName:     "Usuario de prueba",
		Role:         model.RoleVendedor,
		IsActive:     true,
	}
}

func TestRegisterHasheaPasswordYEmiteToken(t *testing.T) {
	repo := newFakeUserRepository()
	tokens := &fakeTokenGenerator{token: "token-de-prueba"}
	svc := NewAuthService(repo, tokens)

	result, err := svc.Register(context.Background(), RegisterInput{
		Email:    "  Maria@Ejemplo.com ",
		Password: "secreto123",
		FullName: "  María Pérez ",
		Phone:    " 1155550000 ",
	})
	if err != nil {
		t.Fatalf("Register() devolvió un error inesperado: %v", err)
	}

	if result.Token != "token-de-prueba" {
		t.Errorf("Token = %q, se esperaba el token del generador", result.Token)
	}
	if result.User.Email != "maria@ejemplo.com" {
		t.Errorf("Email = %q, se esperaba el email normalizado", result.User.Email)
	}
	if result.User.FullName != "María Pérez" || result.User.Phone != "1155550000" {
		t.Errorf("FullName/Phone = %q/%q, se esperaban los valores sin espacios", result.User.FullName, result.User.Phone)
	}
	if result.User.Role != model.RoleVendedor {
		t.Errorf("Role = %q, se esperaba el rol por defecto %q", result.User.Role, model.RoleVendedor)
	}
	if !result.User.IsActive {
		t.Error("IsActive = false, se esperaba true")
	}
	if result.User.ID == uuid.Nil {
		t.Error("ID = uuid.Nil, se esperaba un UUID generado")
	}
	if result.User.PasswordHash == "secreto123" {
		t.Error("PasswordHash contiene la contraseña en texto plano")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(result.User.PasswordHash), []byte("secreto123")); err != nil {
		t.Errorf("el hash guardado no corresponde a la contraseña: %v", err)
	}
	if cost, err := bcrypt.Cost([]byte(result.User.PasswordHash)); err != nil || cost != BcryptCost {
		t.Errorf("costo de bcrypt = %d (err=%v), se esperaba %d", cost, err, BcryptCost)
	}
	if _, ok := repo.users["maria@ejemplo.com"]; !ok {
		t.Error("el usuario no quedó guardado en el repositorio")
	}
}

func TestRegisterEmailDuplicado(t *testing.T) {
	repo := newFakeUserRepository()
	existing := newTestUser(t, "secreto123")
	repo.users[existing.Email] = existing
	repo.byID[existing.ID] = existing

	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	_, err := svc.Register(context.Background(), RegisterInput{
		Email:    existing.Email,
		Password: "secreto123",
		FullName: "Otro Usuario",
	})
	if !errors.Is(err, model.ErrEmailAlreadyExists) {
		t.Fatalf("error = %v, se esperaba model.ErrEmailAlreadyExists", err)
	}
}

func TestRegisterRolInvalido(t *testing.T) {
	repo := newFakeUserRepository()
	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	_, err := svc.Register(context.Background(), RegisterInput{
		Email:    "nuevo@example.com",
		Password: "secreto123",
		FullName: "Nuevo Usuario",
		Role:     "superusuario",
	})
	if !errors.Is(err, model.ErrInvalidRole) {
		t.Fatalf("error = %v, se esperaba model.ErrInvalidRole", err)
	}
	if len(repo.users) != 0 {
		t.Error("no debería haberse guardado ningún usuario")
	}
}

func TestRegisterPasswordDemasiadoLarga(t *testing.T) {
	repo := newFakeUserRepository()
	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	long := make([]byte, maxPasswordBytes+1)
	for i := range long {
		long[i] = 'a'
	}

	_, err := svc.Register(context.Background(), RegisterInput{
		Email:    "nuevo@example.com",
		Password: string(long),
		FullName: "Nuevo Usuario",
	})
	if !errors.Is(err, model.ErrPasswordTooLong) {
		t.Fatalf("error = %v, se esperaba model.ErrPasswordTooLong", err)
	}
}

func TestLoginExitosoActualizaLastLogin(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser(t, "secreto123")
	repo.users[user.Email] = user
	repo.byID[user.ID] = user

	tokens := &fakeTokenGenerator{token: "token-de-prueba"}
	svc := NewAuthService(repo, tokens)
	fixedNow := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	result, err := svc.Login(context.Background(), "USER@example.com", "secreto123")
	if err != nil {
		t.Fatalf("Login() devolvió un error inesperado: %v", err)
	}

	if result.Token != "token-de-prueba" {
		t.Errorf("Token = %q, se esperaba el token del generador", result.Token)
	}
	if result.User.LastLoginAt == nil || !result.User.LastLoginAt.Equal(fixedNow) {
		t.Errorf("LastLoginAt = %v, se esperaba %v", result.User.LastLoginAt, fixedNow)
	}
	if user.LastLoginAt == nil || !user.LastLoginAt.Equal(fixedNow) {
		t.Errorf("el repositorio no recibió el nuevo LastLoginAt: %v", user.LastLoginAt)
	}
	if len(tokens.calls) != 1 || tokens.calls[0].ID != user.ID {
		t.Error("el token no se generó para el usuario autenticado")
	}
}

func TestLoginPasswordIncorrecta(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser(t, "secreto123")
	repo.users[user.Email] = user
	repo.byID[user.ID] = user

	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	_, err := svc.Login(context.Background(), user.Email, "otra-password")
	if !errors.Is(err, model.ErrInvalidCredentials) {
		t.Fatalf("error = %v, se esperaba model.ErrInvalidCredentials", err)
	}
}

func TestLoginUsuarioInexistente(t *testing.T) {
	svc := NewAuthService(newFakeUserRepository(), &fakeTokenGenerator{token: "token"})

	_, err := svc.Login(context.Background(), "nadie@example.com", "secreto123")
	if !errors.Is(err, model.ErrInvalidCredentials) {
		t.Fatalf("error = %v, se esperaba model.ErrInvalidCredentials", err)
	}
}

func TestLoginUsuarioInactivo(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser(t, "secreto123")
	user.IsActive = false
	repo.users[user.Email] = user
	repo.byID[user.ID] = user

	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	_, err := svc.Login(context.Background(), user.Email, "secreto123")
	if !errors.Is(err, model.ErrUserInactive) {
		t.Fatalf("error = %v, se esperaba model.ErrUserInactive", err)
	}
	if user.LastLoginAt != nil {
		t.Error("no debería actualizarse LastLoginAt de un usuario inactivo")
	}
}

func TestLoginUsuarioSoftDeleted(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser(t, "secreto123")
	user.DeletedAt.Time = time.Now().UTC()
	user.DeletedAt.Valid = true
	repo.users[user.Email] = user
	repo.byID[user.ID] = user

	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	_, err := svc.Login(context.Background(), user.Email, "secreto123")
	if !errors.Is(err, model.ErrInvalidCredentials) {
		t.Fatalf("error = %v, se esperaba model.ErrInvalidCredentials para un usuario eliminado", err)
	}
}

func TestProfileDevuelveUsuarioActivo(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser(t, "secreto123")
	repo.users[user.Email] = user
	repo.byID[user.ID] = user

	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	got, err := svc.Profile(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("Profile() devolvió un error inesperado: %v", err)
	}
	if got.ID != user.ID {
		t.Errorf("ID = %v, se esperaba %v", got.ID, user.ID)
	}
}

func TestProfileUsuarioInexistente(t *testing.T) {
	svc := NewAuthService(newFakeUserRepository(), &fakeTokenGenerator{token: "token"})

	_, err := svc.Profile(context.Background(), uuid.New())
	if !errors.Is(err, model.ErrUserNotFound) {
		t.Fatalf("error = %v, se esperaba model.ErrUserNotFound", err)
	}
}

func TestProfileUsuarioIDNulo(t *testing.T) {
	svc := NewAuthService(newFakeUserRepository(), &fakeTokenGenerator{token: "token"})

	_, err := svc.Profile(context.Background(), uuid.Nil)
	if !errors.Is(err, model.ErrUserNotFound) {
		t.Fatalf("error = %v, se esperaba model.ErrUserNotFound para uuid.Nil", err)
	}
}

func TestProfileUsuarioInactivo(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser(t, "secreto123")
	user.IsActive = false
	repo.users[user.Email] = user
	repo.byID[user.ID] = user

	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	_, err := svc.Profile(context.Background(), user.ID)
	if !errors.Is(err, model.ErrUserInactive) {
		t.Fatalf("error = %v, se esperaba model.ErrUserInactive", err)
	}
}

func TestProfileUsuarioSoftDeleted(t *testing.T) {
	repo := newFakeUserRepository()
	user := newTestUser(t, "secreto123")
	user.DeletedAt.Time = time.Now().UTC()
	user.DeletedAt.Valid = true
	repo.users[user.Email] = user
	repo.byID[user.ID] = user

	svc := NewAuthService(repo, &fakeTokenGenerator{token: "token"})

	_, err := svc.Profile(context.Background(), user.ID)
	if !errors.Is(err, model.ErrUserNotFound) {
		t.Fatalf("error = %v, se esperaba model.ErrUserNotFound para un usuario eliminado", err)
	}
}
