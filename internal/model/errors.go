package model

import "errors"

// Errores de dominio del módulo de usuarios. La capa HTTP los traduce a
// códigos de estado, así que el service nunca conoce HTTP.
var (
	// ErrUserNotFound indica que el usuario no existe o está soft-deleted.
	ErrUserNotFound = errors.New("usuario no encontrado")
	// ErrEmailAlreadyExists indica que ya hay un usuario con ese email.
	ErrEmailAlreadyExists = errors.New("el email ya está registrado")
	// ErrInvalidCredentials se devuelve cuando el email no existe o la
	// contraseña no coincide (mismo error para no filtrar qué falló).
	ErrInvalidCredentials = errors.New("email o contraseña incorrectos")
	// ErrUserInactive se devuelve cuando el usuario está desactivado.
	ErrUserInactive = errors.New("el usuario está desactivado")
	// ErrInvalidRole se devuelve cuando el rol no es uno de los permitidos.
	ErrInvalidRole = errors.New("el rol indicado no es válido")
	// ErrPasswordTooLong se devuelve cuando la contraseña excede lo que bcrypt
	// procesa (72 bytes): a partir de ahí se ignoraría el resto.
	ErrPasswordTooLong = errors.New("la contraseña no puede superar los 72 bytes")
)

// Errores de dominio del módulo de terceros (contacts).
var (
	// ErrContactNotFound indica que el tercero no existe o está soft-deleted.
	ErrContactNotFound = errors.New("tercero no encontrado")
	// ErrContactNameRequired indica que el nombre del tercero está vacío.
	// Name es el único campo obligatorio del modelo.
	ErrContactNameRequired = errors.New("el nombre del tercero es obligatorio")
)
