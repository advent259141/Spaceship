package python

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetectSystemPython(t *testing.T) {
	path := detectSystemPython()
	// On CI or systems without Python, this may return empty — that's fine.
	if path == "" {
		t.Skip("no system Python found, skipping")
	}
	// The detected path should be executable
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("detected Python path %q does not exist: %v", path, err)
	}
}

func TestProbePythonVersion_Valid(t *testing.T) {
	path := detectSystemPython()
	if path == "" {
		t.Skip("no system Python found, skipping")
	}
	version, err := probePythonVersion(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version == "" {
		t.Fatal("expected non-empty version string")
	}
	t.Logf("detected Python version: %s", version)
}

func TestProbePythonVersion_EmptyPath(t *testing.T) {
	_, err := probePythonVersion("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestProbePythonVersion_InvalidPath(t *testing.T) {
	_, err := probePythonVersion("/nonexistent/python999")
	if err == nil {
		t.Fatal("expected error for non-existent python binary")
	}
}

func TestVenvPythonBinary_Linux(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("testing Linux path on Windows")
	}
	result := venvPythonBinary("/home/user/.spaceship/venv")
	expected := "/home/user/.spaceship/venv/bin/python"
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestVenvPythonBinary_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("testing Windows path on non-Windows")
	}
	result := venvPythonBinary(`C:\Users\test\.spaceship\venv`)
	expected := filepath.Join(`C:\Users\test\.spaceship\venv`, "Scripts", "python.exe")
	if result != expected {
		t.Fatalf("expected %q, got %q", expected, result)
	}
}

func TestDefaultVenvDir(t *testing.T) {
	dir := defaultVenvDir()
	if dir == "" {
		t.Fatal("expected non-empty default venv dir")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("expected absolute path, got %q", dir)
	}
}

func TestSetup_ExplicitPathValid(t *testing.T) {
	path := detectSystemPython()
	if path == "" {
		t.Skip("no system Python found, skipping")
	}
	env := Setup(Options{PythonPath: path})
	if !env.Available {
		t.Fatal("expected Available=true with valid explicit path")
	}
	if env.PythonPath != path {
		t.Fatalf("expected PythonPath=%q, got %q", path, env.PythonPath)
	}
	if env.IsVenv {
		t.Fatal("expected IsVenv=false for explicit path")
	}
	if env.Version == "" {
		t.Fatal("expected non-empty version")
	}
}

func TestSetup_ExplicitPathInvalid(t *testing.T) {
	env := Setup(Options{PythonPath: "/nonexistent/python999"})
	if env.Available {
		t.Fatal("expected Available=false with invalid explicit path")
	}
}

func TestSetup_SkipVenv(t *testing.T) {
	path := detectSystemPython()
	if path == "" {
		t.Skip("no system Python found, skipping")
	}
	env := Setup(Options{SkipVenv: true})
	if !env.Available {
		t.Fatal("expected Available=true")
	}
	if env.IsVenv {
		t.Fatal("expected IsVenv=false when SkipVenv is true")
	}
}

func TestSetup_CreateVenv(t *testing.T) {
	path := detectSystemPython()
	if path == "" {
		t.Skip("no system Python found, skipping")
	}

	// Use a temp dir for the venv
	venvDir := filepath.Join(t.TempDir(), "test_venv")

	env := Setup(Options{VenvDir: venvDir})
	if !env.Available {
		t.Fatal("expected Available=true")
	}
	if !env.IsVenv {
		t.Fatal("expected IsVenv=true after venv creation")
	}
	if env.VenvDir != venvDir {
		t.Fatalf("expected VenvDir=%q, got %q", venvDir, env.VenvDir)
	}
	// Verify the venv Python binary exists
	if _, err := os.Stat(env.PythonPath); err != nil {
		t.Fatalf("venv python binary not found: %v", err)
	}
}

func TestSetup_ReuseExistingVenv(t *testing.T) {
	path := detectSystemPython()
	if path == "" {
		t.Skip("no system Python found, skipping")
	}

	venvDir := filepath.Join(t.TempDir(), "reuse_venv")

	// First setup creates the venv
	env1 := Setup(Options{VenvDir: venvDir})
	if !env1.Available || !env1.IsVenv {
		t.Fatal("first setup should create a venv")
	}

	// Second setup should reuse it
	env2 := Setup(Options{VenvDir: venvDir})
	if !env2.Available || !env2.IsVenv {
		t.Fatal("second setup should reuse existing venv")
	}
	if env2.PythonPath != env1.PythonPath {
		t.Fatalf("expected same PythonPath on reuse, got %q vs %q", env1.PythonPath, env2.PythonPath)
	}
}
