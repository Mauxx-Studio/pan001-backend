package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/pan001/backend/internal/model"
)

func testUser() *model.User {
	return &model.User{
		ID:    uuid.MustParse("6f1b1f0e-3f8e-4a1c-9d0b-2b7a1c5e9f10"),
		Email: "maria@ejemplo.com",
		Role:  model.RoleVendedor,
	}
}

func TestGenerateYParse(t *testing.T) {
	manager := NewJWTManager("secreto-de-pruebas", DefaultTokenTTL)
	user := testUser()

	token, err := manager.Generate(user)
	if err != nil {
		t.Fatalf("Generate() devolvió un error inesperado: %v", err)
	}

	claims, err := manager.Parse(token)
	if err != nil {
		t.Fatalf("Parse() devolvió un error inesperado: %v", err)
	}

	if claims.UserID != user.ID.String() {
		t.Errorf("UserID = %q, se esperaba %q", claims.UserID, user.ID)
	}
	if claims.Email != user.Email {
		t.Errorf("Email = %q, se esperaba %q", claims.Email, user.Email)
	}
	if claims.Role != user.Role {
		t.Errorf("Role = %q, se esperaba %q", claims.Role, user.Role)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("ExpiresAt = nil, el token debe incluir exp")
	}

	// La vigencia configurada es de 24 horas.
	if got := claims.ExpiresAt.Sub(claims.IssuedAt.Time); got != 24*time.Hour {
		t.Errorf("vigencia = %v, se esperaban 24 h", got)
	}
}

func TestParseTokenExpirado(t *testing.T) {
	manager := NewJWTManager("secreto-de-pruebas", time.Hour)
	manager.now = func() time.Time { return time.Now().Add(-2 * time.Hour) }

	token, err := manager.Generate(testUser())
	if err != nil {
		t.Fatalf("Generate() devolvió un error inesperado: %v", err)
	}

	if _, err := manager.Parse(token); err == nil {
		t.Fatal("Parse() devolvió nil, se esperaba un error por token expirado")
	}
}

func TestParseFirmaInvalida(t *testing.T) {
	token, err := NewJWTManager("secreto-de-pruebas", time.Hour).Generate(testUser())
	if err != nil {
		t.Fatalf("Generate() devolvió un error inesperado: %v", err)
	}

	if _, err := NewJWTManager("otro-secreto", time.Hour).Parse(token); err == nil {
		t.Fatal("Parse() devolvió nil, se esperaba un error por firma inválida")
	}
}

func TestParseAlgoritmoNoPermitido(t *testing.T) {
	// Un token "alg: none" no debe ser aceptado aunque no tenga firma.
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"user_id": uuid.NewString(),
		"exp":     time.Now().Add(time.Hour).Unix(),
	})
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("no se pudo construir el token de prueba: %v", err)
	}

	if _, err := NewJWTManager("secreto-de-pruebas", time.Hour).Parse(signed); err == nil {
		t.Fatal("Parse() devolvió nil, se esperaba un error por algoritmo no permitido")
	}
}
