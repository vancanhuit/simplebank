package util

import "testing"

func TestIsSupportedCurrency(t *testing.T) {
	for _, c := range []string{USD, EUR, VND} {
		if !IsSupportedCurrency(c) {
			t.Errorf("%s should be supported", c)
		}
	}
	if IsSupportedCurrency("XYZ") {
		t.Error("XYZ should not be supported")
	}
}
