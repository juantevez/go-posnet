package validator

// LuhnCheck verifica que el PAN tenga un dígito verificador válido (algoritmo de Luhn).
// Se usa como primera barrera antes de enviar al adquirente.
// No valida que la tarjeta exista — solo que el número sea matemáticamente válido.
func LuhnCheck(pan string) bool {
	if len(pan) < 13 || len(pan) > 19 {
		return false
	}
	sum := 0
	nDigits := len(pan)
	parity := nDigits % 2

	for i := 0; i < nDigits; i++ {
		d := int(pan[i] - '0')
		if d < 0 || d > 9 {
			return false // carácter no numérico
		}
		if i%2 == parity {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}
