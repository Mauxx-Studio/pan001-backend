# Makefile con las tareas habituales del proyecto (API REST con Fiber).
# Ejecuta `make help` para ver la lista de comandos disponibles.

APP_NAME := api
CMD_PATH := ./cmd/api
BIN_DIR  := bin

.DEFAULT_GOAL := help

.PHONY: help deps run build test vet fmt tidy ci clean

help: ## Muestra esta ayuda
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

deps: ## Descarga y verifica las dependencias
	go mod download
	go mod verify

run: ## Ejecuta la API en modo desarrollo
	go run $(CMD_PATH)

build: ## Compila el binario en bin/
	@mkdir -p $(BIN_DIR)
	go build -trimpath -ldflags="-s -w" -o $(BIN_DIR)/$(APP_NAME) $(CMD_PATH)

test: ## Ejecuta las pruebas
	go test ./... -race -cover

vet: ## Ejecuta el analizador estático de Go
	go vet ./...

fmt: ## Formatea el código
	go fmt ./...

tidy: ## Sincroniza go.mod y go.sum
	go mod tidy

ci: tidy vet test build ## Ejecuta las comprobaciones que correría el CI

clean: ## Elimina los artefactos generados
	rm -rf $(BIN_DIR) coverage.out coverage.html
