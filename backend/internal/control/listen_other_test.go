//go:build !windows

package control

import (
	"path/filepath"
	"testing"
)

func TestSocketPathServesTheInstalledEndpointWhereNoInstanceIsNamed(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv(EnvInstance, "")

	path, err := socketPath()
	if err != nil {
		t.Fatalf("socketPath() failed: %v", err)
	}

	want := filepath.Join(runtime, "mirrorme", "control-v1.sock")
	if path != want {
		t.Errorf("socketPath() = %q, want %q", path, want)
	}
}

func TestSocketPathSeparatesAnInstanceFromTheInstalledEndpoint(t *testing.T) {
	runtime := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", runtime)
	t.Setenv(EnvInstance, "dev")

	path, err := socketPath()
	if err != nil {
		t.Fatalf("socketPath() failed: %v", err)
	}

	want := filepath.Join(runtime, "mirrorme", "control-v1-dev.sock")
	if path != want {
		t.Errorf("socketPath() = %q, want %q", path, want)
	}
}
