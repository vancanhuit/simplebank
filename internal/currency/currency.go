package currency

const (
	USD = "USD"
	EUR = "EUR"
	VND = "VND"
)

// MaxSafeMinorUnits is the largest integer that JavaScript can represent exactly
// in a JSON number: 2^53 - 1. All monetary values must stay within this bound to
// ensure the frontend receives lossless JSON numbers.
const MaxSafeMinorUnits int64 = 1<<53 - 1

func IsSupported(currency string) bool {
	switch currency {
	case USD, EUR, VND:
		return true
	}
	return false
}
