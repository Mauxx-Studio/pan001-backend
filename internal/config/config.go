// Package config carga y valida la configuración de la aplicación a partir de
// variables de entorno, con soporte opcional para un archivo .env en desarrollo
// local (ver .env.example).
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

const (
	// envFile es el archivo que se carga (si existe) al arrancar la aplicación.
	envFile = ".env"

	// defaultPort es el puerto HTTP usado cuando PORT no está definida.
	defaultPort = "8080"
	// developmentEnv identifica el entorno de desarrollo local.
	developmentEnv = "development"
	// productionEnv identifica el entorno productivo.
	productionEnv = "production"
	// defaultAppEnv es el entorno usado cuando APP_ENV no está definida.
	defaultAppEnv = developmentEnv

	// defaultJWTSecret es el secreto de desarrollo usado cuando JWT_SECRET no
	// está definida. En producción JWT_SECRET es obligatoria (ver validate).
	defaultJWTSecret = "dev-secret-cambiar-en-produccion"

	// defaultDBHost es el host de PostgreSQL usado por defecto.
	defaultDBHost = "localhost"
	// defaultDBPort es el puerto de PostgreSQL usado por defecto.
	defaultDBPort = "5432"
	// defaultDBSSLMode es el modo SSL usado por defecto (desarrollo local).
	defaultDBSSLMode = "disable"
)

// Config agrupa toda la configuración de la aplicación.
type Config struct {
	App      App
	Database Database
	Auth     Auth
}

// App contiene la configuración del servidor HTTP.
type App struct {
	// Port es el puerto donde escucha la API.
	Port string
	// Env es el entorno de ejecución: development | staging | production.
	Env string
}

// IsDevelopment indica si la aplicación corre en desarrollo local.
func (a App) IsDevelopment() bool {
	return a.Env == developmentEnv
}

// Database contiene la configuración de la conexión a PostgreSQL.
type Database struct {
	Host     string
	Port     int
	User     string
	Password string
	Name     string
	SSLMode  string
}

// Auth contiene la configuración de autenticación.
type Auth struct {
	// JWTSecret es la clave HMAC con la que se firman los access tokens.
	JWTSecret string
}

// UsingDefaultSecret indica si se está usando el secreto de desarrollo, es
// decir, si JWT_SECRET no fue definida. Sirve para advertirlo en los logs.
func (a Auth) UsingDefaultSecret() bool {
	return a.JWTSecret == defaultJWTSecret
}

// DSN arma la cadena de conexión que espera el driver de PostgreSQL.
func (d Database) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode,
	)
}

// String devuelve una representación de la conexión segura para logs: nunca
// incluye la contraseña.
func (d Database) String() string {
	return fmt.Sprintf("%s@%s:%d/%s?sslmode=%s", d.User, d.Host, d.Port, d.Name, d.SSLMode)
}

// Load lee la configuración desde las variables de entorno. Antes de leerlas
// intenta cargar el archivo .env (si existe); las variables ya definidas en el
// entorno tienen prioridad, así que en producción basta con inyectarlas.
func Load() (*Config, error) {
	if err := loadEnvFile(); err != nil {
		return nil, err
	}

	dbPort, err := envInt("DB_PORT", defaultDBPort)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		App: App{
			Port: envOr("PORT", defaultPort),
			Env:  envOr("APP_ENV", defaultAppEnv),
		},
		Database: Database{
			Host:     envOr("DB_HOST", defaultDBHost),
			Port:     dbPort,
			User:     os.Getenv("DB_USER"),
			Password: os.Getenv("DB_PASSWORD"),
			Name:     os.Getenv("DB_NAME"),
			SSLMode:  envOr("DB_SSLMODE", defaultDBSSLMode),
		},
		Auth: Auth{
			JWTSecret: envOr("JWT_SECRET", defaultJWTSecret),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// validate comprueba que las variables obligatorias estén presentes.
func (c *Config) validate() error {
	missing := make([]string, 0, 2)
	if c.Database.User == "" {
		missing = append(missing, "DB_USER")
	}
	if c.Database.Name == "" {
		missing = append(missing, "DB_NAME")
	}
	if len(missing) > 0 {
		return fmt.Errorf("faltan variables de entorno obligatorias: %s", strings.Join(missing, ", "))
	}

	// En desarrollo se permite el secreto por defecto; en producción firmar los
	// tokens con un secreto conocido sería un agujero de seguridad.
	if c.App.Env == productionEnv && c.Auth.UsingDefaultSecret() {
		return errors.New("JWT_SECRET es obligatoria cuando APP_ENV=production")
	}

	return nil
}

// loadEnvFile carga el archivo .env cuando existe. Si no existe no es un error:
// en producción la configuración suele venir del entorno del contenedor.
func loadEnvFile() error {
	err := godotenv.Load(envFile)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("cargando %s: %w", envFile, err)
}

// envOr devuelve el valor de la variable key o fallback si está vacía.
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// envInt devuelve el valor de la variable key convertido a entero.
func envInt(key, fallback string) (int, error) {
	raw := envOr(key, fallback)

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("la variable %s debe ser un número, se obtuvo %q", key, raw)
	}
	if value <= 0 || value > 65535 {
		return 0, fmt.Errorf("la variable %s debe ser un puerto válido, se obtuvo %d", key, value)
	}

	return value, nil
}
