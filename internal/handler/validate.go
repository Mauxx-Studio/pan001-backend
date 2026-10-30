package handler

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"

	"github.com/pan001/backend/internal/model"
)

// fieldLabels traduce los nombres de campo de Go a los nombres que ve el
// cliente en el JSON, para que los mensajes de validación sean claros.
var fieldLabels = map[string]string{
	"Email":    "email",
	"Password": "password",
	"FullName": "full_name",
	"Phone":    "phone",
	"Role":     "role",
}

// newValidator crea el validador con las reglas propias del dominio.
func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())

	// La etiqueta `role` reutiliza los roles del dominio en lugar de repetir la
	// lista en cada struct tag.
	if err := v.RegisterValidation("role", validateRole); err != nil {
		// Solo falla si el tag o la función son inválidos: es un error de código.
		panic(fmt.Sprintf("registrando la validación role: %v", err))
	}

	return v
}

// validateRole acepta únicamente los roles definidos en el dominio.
func validateRole(fl validator.FieldLevel) bool {
	return model.IsValidRole(fl.Field().String())
}

// validationMessage convierte los errores de go-playground/validator en un
// mensaje legible en español, con el nombre del campo del JSON.
func validationMessage(err error) string {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return "los datos enviados no son válidos"
	}

	messages := make([]string, 0, len(validationErrs))
	for _, fieldErr := range validationErrs {
		field := fieldLabels[fieldErr.StructField()]
		if field == "" {
			field = strings.ToLower(fieldErr.StructField())
		}
		messages = append(messages, field+" "+ruleMessage(fieldErr))
	}

	return "datos inválidos: " + strings.Join(messages, ", ")
}

// ruleMessage describe en español una regla de validación incumplida.
func ruleMessage(fieldErr validator.FieldError) string {
	switch fieldErr.Tag() {
	case "required":
		return "es obligatorio"
	case "email":
		return "debe ser un email válido"
	case "min":
		if fieldErr.Kind() == reflect.String {
			return "debe tener al menos " + fieldErr.Param() + " caracteres"
		}
		return "debe ser mayor o igual a " + fieldErr.Param()
	case "max":
		if fieldErr.Kind() == reflect.String {
			return "debe tener como máximo " + fieldErr.Param() + " caracteres"
		}
		return "debe ser menor o igual a " + fieldErr.Param()
	case "role":
		return "debe ser uno de: " + strings.Join(model.Roles(), ", ")
	case "oneof":
		return "debe ser uno de: " + strings.ReplaceAll(fieldErr.Param(), " ", ", ")
	default:
		return "no es válido"
	}
}
