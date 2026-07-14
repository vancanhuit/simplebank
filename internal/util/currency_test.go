package util

import "testing"

func TestIsSupportedCurrency(t *testing.T) {
	if !IsSupportedCurrency(USD) {
		t.Error("USD should be supported")
	}
	if IsSupportedCurrency("XYZ") {
		t.Error("XYZ should not be supported")
	}
}
