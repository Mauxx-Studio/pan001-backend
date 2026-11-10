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

	// Campos del módulo de terceros (contacts).
	"Name":         "name",
	"BusinessName": "business_name",
	"TaxID":        "tax_id",
	"WhatsApp":     "whatsapp",
	"Address":      "address",
	"City":         "city",
	"Notes":        "notes",
	"IsClient":     "is_client",
	"IsSupplier":   "is_supplier",
	"IsActive":     "is_active",

	// Campos del módulo de productos (products).
	"SKU":          "sku",
	"Description":  "description",
	"Unit":         "unit",
	"IsSold":       "is_sold",
	"IsPurchased":  "is_purchased",
	"IsProduced":   "is_produced",
	"TrackStock":   "track_stock",
	"CurrentStock": "current_stock",
	"MinStock":     "min_stock",
	"SalePrice":    "sale_price",
	"Cost":         "cost",

	// Campos del módulo de pedidos (orders). Los ítems anidados reutilizan
	// ProductID, Quantity y Notes: validator informa el nombre del campo hoja,
	// así que el mismo label sirve para la cabecera y para las líneas.
	"ID":                "id",
	"Code":              "code",
	"ContactID":         "contact_id",
	"Status":            "status",
	"OrderedAt":         "ordered_at",
	"DeliveryAt":        "delivery_at",
	"DeliveryType":      "delivery_type",
	"DiscountAmount":    "discount_amount",
	"PaymentMethod":     "payment_method",
	"PaymentStatus":     "payment_status",
	"AmountPaid":        "amount_paid",
	"ParentOrderID":     "parent_order_id",
	"Items":             "items",
	"ProductID":         "product_id",
	"Quantity":          "quantity",
	"QuantityDelivered": "quantity_delivered",
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

	// Las etiquetas de los pedidos también salen de las constantes del modelo.
	for name, fn := range map[string]func(validator.FieldLevel) bool{
		"order_status":   validateOrderStatus,
		"delivery_type":  validateDeliveryType,
		"payment_method": validatePaymentMethod,
		"payment_status": validatePaymentStatus,
	} {
		if err := v.RegisterValidation(name, fn); err != nil {
			panic(fmt.Sprintf("registrando la validación %s: %v", name, err))
		}
	}

	return v
}

// validateRole acepta únicamente los roles definidos en el dominio.
func validateRole(fl validator.FieldLevel) bool {
	return model.IsValidRole(fl.Field().String())
}

// validateOrderStatus acepta únicamente los estados de pedido del dominio.
func validateOrderStatus(fl validator.FieldLevel) bool {
	return model.IsValidOrderStatus(fl.Field().String())
}

// validateDeliveryType acepta únicamente pickup o delivery.
func validateDeliveryType(fl validator.FieldLevel) bool {
	return model.IsValidDeliveryType(fl.Field().String())
}

// validatePaymentMethod acepta únicamente los medios de pago del dominio.
func validatePaymentMethod(fl validator.FieldLevel) bool {
	return model.IsValidPaymentMethod(fl.Field().String())
}

// validatePaymentStatus acepta únicamente pending, partial o paid.
func validatePaymentStatus(fl validator.FieldLevel) bool {
	return model.IsValidPaymentStatus(fl.Field().String())
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
	case "order_status":
		return "debe ser uno de: " + strings.Join(model.OrderStatuses(), ", ")
	case "delivery_type":
		return "debe ser uno de: " + strings.Join(model.DeliveryTypes(), ", ")
	case "payment_method":
		return "debe ser uno de: " + strings.Join(model.PaymentMethods(), ", ")
	case "payment_status":
		return "debe ser uno de: " + strings.Join(model.PaymentStatuses(), ", ")
	case "uuid":
		return "debe ser un UUID válido"
	case "oneof":
		return "debe ser uno de: " + strings.ReplaceAll(fieldErr.Param(), " ", ", ")
	default:
		return "no es válido"
	}
}
