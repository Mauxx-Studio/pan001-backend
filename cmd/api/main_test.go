package main

import (
	"net/http"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/pan001/backend/internal/config"
)

// TestNewAppCORS comprueba que el middleware CORS global se monta y responde a
// las peticiones preflight (OPTIONS) con los headers Access-Control-* correctos.
// El preflight se resuelve antes de llegar a cualquier handler, por eso se
// puede construir la app con db nil (solo /health la usaría en un GET real).
func TestNewAppCORS(t *testing.T) {
	const origin = "https://app.pan001.com"

	cfg := &config.Config{App: config.App{AllowedOrigins: []string{origin}}}
	app := newApp(nil, cfg)

	req, err := http.NewRequest(http.MethodOptions, "/health", nil)
	if err != nil {
		t.Fatalf("no se pudo crear la petición: %v", err)
	}
	req.Header.Set(fiber.HeaderOrigin, origin)
	req.Header.Set(fiber.HeaderAccessControlRequestMethod, fiber.MethodGet)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test devolvió un error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, se esperaba %d", resp.StatusCode, http.StatusNoContent)
	}
	if got := resp.Header.Get(fiber.HeaderAccessControlAllowOrigin); got != origin {
		t.Errorf("Access-Control-Allow-Origin = %q, se esperaba %q", got, origin)
	}
	if got := resp.Header.Get(fiber.HeaderAccessControlAllowMethods); got == "" {
		t.Error("Access-Control-Allow-Methods vacío, se esperaban métodos permitidos")
	}
}
