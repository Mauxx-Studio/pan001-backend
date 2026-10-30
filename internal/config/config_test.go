package config

import (
	"strings"
	"testing"
)

// resetEnv deja el entorno limpio de las variables que lee Load para que cada
// caso arranque desde cero (t.Setenv restaura el valor original al terminar).
func resetEnv(t *testing.T) {
	t.Helper()

	for _, key := range []string{
		"PORT", "APP_ENV", "JWT_SECRET",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
	} {
		t.Setenv(key, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	resetEnv(t)
	t.Setenv("DB_USER", "mauri")
	t.Setenv("DB_NAME", "miapp")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() devolvió un error inesperado: %v", err)
	}

	if cfg.App.Port != defaultPort {
		t.Errorf("App.Port = %q, se esperaba %q", cfg.App.Port, defaultPort)
	}
	if cfg.App.Env != defaultAppEnv {
		t.Errorf("App.Env = %q, se esperaba %q", cfg.App.Env, defaultAppEnv)
	}
	if !cfg.App.IsDevelopment() {
		t.Error("IsDevelopment() = false, se esperaba true con APP_ENV por defecto")
	}

	// En desarrollo, si no se define JWT_SECRET se usa el secreto de desarrollo.
	if !cfg.Auth.UsingDefaultSecret() {
		t.Errorf("Auth.JWTSecret = %q, se esperaba el secreto de desarrollo", cfg.Auth.JWTSecret)
	}

	if cfg.Database.Host != defaultDBHost {
		t.Errorf("Database.Host = %q, se esperaba %q", cfg.Database.Host, defaultDBHost)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Database.Port = %d, se esperaba 5432", cfg.Database.Port)
	}
	if cfg.Database.SSLMode != defaultDBSSLMode {
		t.Errorf("Database.SSLMode = %q, se esperaba %q", cfg.Database.SSLMode, defaultDBSSLMode)
	}
}

func TestLoadLeeVariablesDeEntorno(t *testing.T) {
	resetEnv(t)
	t.Setenv("PORT", "9090")
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_HOST", "db.interno")
	t.Setenv("DB_PORT", "6543")
	t.Setenv("DB_USER", "pan001")
	t.Setenv("DB_PASSWORD", "secreto")
	t.Setenv("DB_NAME", "pan001_db")
	t.Setenv("DB_SSLMODE", "require")
	t.Setenv("JWT_SECRET", "secreto-de-pruebas")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() devolvió un error inesperado: %v", err)
	}

	if cfg.App.Port != "9090" || cfg.App.Env != "production" || cfg.App.IsDevelopment() {
		t.Errorf("App = %+v, valores inesperados", cfg.App)
	}

	if cfg.Auth.JWTSecret != "secreto-de-pruebas" || cfg.Auth.UsingDefaultSecret() {
		t.Errorf("Auth = %+v, se esperaba el JWT_SECRET del entorno", cfg.Auth)
	}

	wantDSN := "host=db.interno port=6543 user=pan001 password=secreto dbname=pan001_db sslmode=require"
	if got := cfg.Database.DSN(); got != wantDSN {
		t.Errorf("DSN() = %q, se esperaba %q", got, wantDSN)
	}
}

func TestLoadVariablesObligatorias(t *testing.T) {
	tests := []struct {
		name    string
		user    string
		dbName  string
		wantErr string
	}{
		{name: "sin DB_USER", dbName: "miapp", wantErr: "DB_USER"},
		{name: "sin DB_NAME", user: "mauri", wantErr: "DB_NAME"},
		{name: "sin ninguna", wantErr: "DB_USER, DB_NAME"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resetEnv(t)
			t.Setenv("DB_USER", tc.user)
			t.Setenv("DB_NAME", tc.dbName)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() devolvió nil, se esperaba un error de validación")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, se esperaba que mencione %q", err, tc.wantErr)
			}
		})
	}
}

func TestLoadPuertoInvalido(t *testing.T) {
	resetEnv(t)
	t.Setenv("DB_USER", "mauri")
	t.Setenv("DB_NAME", "miapp")
	t.Setenv("DB_PORT", "no-es-un-puerto")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() devolvió nil, se esperaba un error por DB_PORT inválido")
	}
	if !strings.Contains(err.Error(), "DB_PORT") {
		t.Errorf("error = %q, se esperaba que mencione DB_PORT", err)
	}
}

func TestLoadJWTSecretObligatoriaEnProduccion(t *testing.T) {
	resetEnv(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_USER", "mauri")
	t.Setenv("DB_NAME", "miapp")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() devolvió nil, se esperaba un error por JWT_SECRET faltante en producción")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Errorf("error = %q, se esperaba que mencione JWT_SECRET", err)
	}
}

func TestDatabaseStringNoExponePassword(t *testing.T) {
	db := Database{
		Host:     "localhost",
		Port:     5432,
		User:     "mauri",
		Password: "super-secreta",
		Name:     "miapp",
		SSLMode:  "disable",
	}

	got := db.String()
	if strings.Contains(got, db.Password) {
		t.Errorf("String() = %q, no debe incluir la contraseña", got)
	}

	want := "mauri@localhost:5432/miapp?sslmode=disable"
	if got != want {
		t.Errorf("String() = %q, se esperaba %q", got, want)
	}
}
