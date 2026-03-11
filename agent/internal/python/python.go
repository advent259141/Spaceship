package python

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Environment holds the resolved Python environment state.
type Environment struct {
	Available  bool   // Whether a usable Python was found
	PythonPath string // Absolute path to the selected Python binary
	IsVenv     bool   // Whether PythonPath points to a managed venv
	VenvDir    string // Path to the venv directory (empty if not using venv)
	Version    string // Python version string (e.g. "3.12.1")
}

// Options for configuring the Python environment setup.
type Options struct {
	// PythonPath overrides auto-detection. If set and valid, will be used directly.
	PythonPath string
	// VenvDir overrides the default venv location (~/.spaceship/venv).
	VenvDir string
	// SkipVenv disables automatic venv creation; use system Python directly.
	SkipVenv bool
	// Logger for diagnostic output.
	Logger *slog.Logger
}

// Setup detects or provisions a Python environment for the agent.
//
// Resolution order:
//  1. If Options.PythonPath is set and valid → use it directly (no venv).
//  2. Check if a managed venv already exists at VenvDir → reuse it.
//  3. Detect system Python (python3, python) → create venv.
//  4. If nothing found → return Environment{Available: false}.
func Setup(opts Options) Environment {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// 1. Explicit override
	if opts.PythonPath != "" {
		version, err := probePythonVersion(opts.PythonPath)
		if err != nil {
			logger.Warn("configured python_path is not valid",
				"path", opts.PythonPath,
				"error", err,
			)
			return Environment{Available: false}
		}
		logger.Info("using configured python",
			"path", opts.PythonPath,
			"version", version,
		)
		return Environment{
			Available:  true,
			PythonPath: opts.PythonPath,
			IsVenv:     false,
			Version:    version,
		}
	}

	venvDir := opts.VenvDir
	if venvDir == "" {
		venvDir = defaultVenvDir()
	}

	// 2. Check existing venv
	venvPython := venvPythonBinary(venvDir)
	if version, err := probePythonVersion(venvPython); err == nil {
		logger.Info("reusing existing venv",
			"venv_dir", venvDir,
			"python", venvPython,
			"version", version,
		)
		return Environment{
			Available:  true,
			PythonPath: venvPython,
			IsVenv:     true,
			VenvDir:    venvDir,
			Version:    version,
		}
	}

	// 3. Detect system Python
	systemPython := detectSystemPython()
	if systemPython == "" {
		logger.Info("python not found on this system, python capability disabled")
		return Environment{Available: false}
	}

	systemVersion, err := probePythonVersion(systemPython)
	if err != nil {
		logger.Warn("detected python binary but cannot probe version",
			"path", systemPython,
			"error", err,
		)
		return Environment{Available: false}
	}
	logger.Info("detected system python",
		"path", systemPython,
		"version", systemVersion,
	)

	// If venv creation is disabled, use system Python directly
	if opts.SkipVenv {
		return Environment{
			Available:  true,
			PythonPath: systemPython,
			IsVenv:     false,
			Version:    systemVersion,
		}
	}

	// 4. Create venv
	if err := createVenv(systemPython, venvDir, logger); err != nil {
		logger.Warn("failed to create venv, falling back to system python",
			"error", err,
		)
		return Environment{
			Available:  true,
			PythonPath: systemPython,
			IsVenv:     false,
			Version:    systemVersion,
		}
	}

	venvVersion, err := probePythonVersion(venvPython)
	if err != nil {
		logger.Warn("venv created but python binary not found, falling back to system python",
			"venv_dir", venvDir,
			"error", err,
		)
		return Environment{
			Available:  true,
			PythonPath: systemPython,
			IsVenv:     false,
			Version:    systemVersion,
		}
	}

	logger.Info("created new venv",
		"venv_dir", venvDir,
		"python", venvPython,
		"version", venvVersion,
	)
	return Environment{
		Available:  true,
		PythonPath: venvPython,
		IsVenv:     true,
		VenvDir:    venvDir,
		Version:    venvVersion,
	}
}

// detectSystemPython checks common Python binary names in PATH.
func detectSystemPython() string {
	candidates := []string{"python3", "python"}
	for _, name := range candidates {
		path, err := exec.LookPath(name)
		if err == nil {
			return path
		}
	}
	return ""
}

// probePythonVersion runs `python --version` and returns the version string.
func probePythonVersion(pythonPath string) (string, error) {
	if pythonPath == "" {
		return "", errors.New("empty python path")
	}
	cmd := exec.Command(pythonPath, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run %s --version: %w", pythonPath, err)
	}
	// Output is like "Python 3.12.1\n"
	version := strings.TrimSpace(string(out))
	version = strings.TrimPrefix(version, "Python ")
	return version, nil
}

// createVenv creates a new Python virtual environment.
func createVenv(pythonPath, venvDir string, logger *slog.Logger) error {
	// Ensure parent directory exists
	parent := filepath.Dir(venvDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("cannot create parent dir: %w", err)
	}

	logger.Info("creating python venv",
		"python", pythonPath,
		"venv_dir", venvDir,
	)
	cmd := exec.Command(pythonPath, "-m", "venv", venvDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("python -m venv failed: %w", err)
	}
	return nil
}

// venvPythonBinary returns the path to the Python binary inside a venv.
func venvPythonBinary(venvDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", "python.exe")
	}
	return filepath.Join(venvDir, "bin", "python")
}

// defaultVenvDir returns the default venv directory path.
func defaultVenvDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".spaceship", "venv")
}
