package currency

import "testing"

func TestIsSupported(t *testing.T) {
	t.Parallel()
	for _, c := range []string{USD, EUR, VND} {
		if !IsSupported(c) {
			t.Errorf("%s should be supported", c)
		}
	}
	if IsSupported("XYZ") {
		t.Error("XYZ should not be supported")
	}
}
