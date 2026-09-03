package control

import "testing"

func TestInstanceSuffixIsEmptyWhereNoInstanceIsNamed(t *testing.T) {
	t.Setenv(EnvInstance, "")

	if got := instanceSuffix(); got != "" {
		t.Errorf("instanceSuffix() = %q, want the installed endpoint's empty suffix", got)
	}
}

func TestInstanceSuffixNamesTheInstance(t *testing.T) {
	t.Setenv(EnvInstance, "dev")

	if got := instanceSuffix(); got != "-dev" {
		t.Errorf("instanceSuffix() = %q, want %q", got, "-dev")
	}
}
