package currency

const (
	USD = "USD"
	EUR = "EUR"
	VND = "VND"
)

func IsSupported(currency string) bool {
	switch currency {
	case USD, EUR, VND:
		return true
	}
	return false
}
