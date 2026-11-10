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

// Errores de dominio del módulo de productos (products).
var (
	// ErrProductNotFound indica que el producto no existe o está soft-deleted.
	ErrProductNotFound = errors.New("producto no encontrado")
	// ErrProductNameRequired indica que el nombre del producto está vacío.
	ErrProductNameRequired = errors.New("el nombre del producto es obligatorio")
	// ErrProductUnitRequired indica que la unidad de medida está vacía.
	// Name y Unit son los únicos campos obligatorios del modelo.
	ErrProductUnitRequired = errors.New("la unidad del producto es obligatoria")
)

// Errores de dominio del módulo de pedidos (orders).
var (
	// ErrOrderNotFound indica que el pedido no existe o está soft-deleted.
	ErrOrderNotFound = errors.New("pedido no encontrado")
	// ErrOrderContactRequired indica que falta el tercero del pedido.
	ErrOrderContactRequired = errors.New("el tercero del pedido es obligatorio")
	// ErrOrderItemsRequired indica que un pedido que no es borrador quedó sin
	// líneas: todo pedido en firme debe tener al menos un ítem.
	ErrOrderItemsRequired = errors.New("el pedido debe tener al menos un ítem")
	// ErrOrderStatusInvalid indica que el estado no es uno de los permitidos.
	ErrOrderStatusInvalid = errors.New("el estado del pedido no es válido")
	// ErrOrderDeliveryTypeInvalid indica que el tipo de entrega no es válido.
	ErrOrderDeliveryTypeInvalid = errors.New("el tipo de entrega no es válido")
	// ErrOrderPaymentMethodInvalid indica que el medio de pago no es válido.
	ErrOrderPaymentMethodInvalid = errors.New("el medio de pago no es válido")
	// ErrOrderPaymentStatusInvalid indica que el estado de pago no es válido.
	ErrOrderPaymentStatusInvalid = errors.New("el estado de pago no es válido")
	// ErrOrderDiscountInvalid indica un descuento negativo o mayor que el
	// subtotal (dejaría un importe final negativo).
	ErrOrderDiscountInvalid = errors.New("el descuento no puede ser negativo ni superar el subtotal del pedido")
	// ErrOrderAmountPaidInvalid indica un importe cobrado negativo.
	ErrOrderAmountPaidInvalid = errors.New("el importe cobrado no puede ser negativo")
	// ErrOrderItemProductRequired indica que una línea llegó sin producto.
	ErrOrderItemProductRequired = errors.New("el producto del ítem es obligatorio")
	// ErrOrderItemQuantityInvalid indica una cantidad nula, negativa o menor a
	// lo ya entregado de esa línea.
	ErrOrderItemQuantityInvalid = errors.New("la cantidad del ítem debe ser mayor que cero y no menor a lo ya entregado")
	// ErrOrderItemDeliveredInvalid indica una cantidad entregada negativa o
	// mayor que la cantidad pedida.
	ErrOrderItemDeliveredInvalid = errors.New("la cantidad entregada del ítem debe estar entre cero y la cantidad pedida")
	// ErrOrderItemNotInOrder indica que el id de un ítem no pertenece al pedido.
	ErrOrderItemNotInOrder = errors.New("el ítem indicado no pertenece al pedido")
	// ErrOrderItemDuplicated indica que el mismo ítem vino dos veces en el body.
	ErrOrderItemDuplicated = errors.New("el ítem aparece más de una vez en el pedido")
	// ErrOrderItemDeliveredLocked impide quitar una línea que ya tiene
	// entregas registradas: primero habría que revertir la entrega.
	ErrOrderItemDeliveredLocked = errors.New("no se puede quitar un ítem con cantidad entregada")
	// ErrOrderItemNotFound indica que la línea desapareció entre la lectura y
	// la escritura (carrera con otra petición).
	ErrOrderItemNotFound = errors.New("ítem del pedido no encontrado")
	// ErrOrderProductPriceInvalid indica que el producto no tiene un precio de
	// venta positivo y por lo tanto no se puede cotizar la línea.
	ErrOrderProductPriceInvalid = errors.New("el producto no tiene un precio de venta válido")
	// ErrOrderCodeConflict indica que la secuencia diaria del código chocó con
	// el índice único (otra petición ganó la carrera). El service reintenta.
	ErrOrderCodeConflict = errors.New("conflicto al generar el código del pedido")
)
