package machineid

import (
	"crypto/sha1"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// spaceshipNamespace is a fixed UUID namespace for generating deterministic node IDs.
// All spaceship agents share this namespace.
var spaceshipNamespace = [16]byte{
	0xa1, 0xb2, 0xc3, 0xd4, 0xe5, 0xf6, 0x47, 0x08,
	0x89, 0x0a, 0x1b, 0x2c, 0x3d, 0x4e, 0x5f, 0x60,
}

// NodeID returns a deterministic UUID v5 derived from the OS-level machine
// identifier. The same physical/virtual machine always produces the same ID,
// even if the spaceship agent is completely deleted and reinstalled.
//
// Only an OS reinstall (which regenerates the machine-id) would change it.
func NodeID() (string, error) {
	mid, err := readMachineID()
	if err != nil {
		return "", fmt.Errorf("failed to read machine-id: %w", err)
	}
	return uuidV5(spaceshipNamespace, mid), nil
}

// readMachineID reads the OS-level machine identifier.
//   - Linux:   /etc/machine-id
//   - Windows: HKLM\SOFTWARE\Microsoft\Cryptography\MachineGuid (via reg query)
//   - macOS:   IOPlatformUUID (via ioreg)
func readMachineID() (string, error) {
	switch runtime.GOOS {
	case "linux":
		return readLinuxMachineID()
	case "darwin":
		return readDarwinMachineID()
	case "windows":
		return readWindowsMachineID()
	default:
		// Fallback: use hostname (less unique but better than nothing).
		h, err := os.Hostname()
		if err != nil {
			return "", fmt.Errorf("unsupported OS %q and hostname unavailable: %w", runtime.GOOS, err)
		}
		return "hostname:" + h, nil
	}
}

func readLinuxMachineID() (string, error) {
	// Try /etc/machine-id first (systemd), then /var/lib/dbus/machine-id.
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		data, err := os.ReadFile(path)
		if err == nil {
			id := strings.TrimSpace(string(data))
			if id != "" {
				return id, nil
			}
		}
	}
	return "", fmt.Errorf("machine-id not found in /etc/machine-id or /var/lib/dbus/machine-id")
}

func readDarwinMachineID() (string, error) {
	out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
	if err != nil {
		return "", fmt.Errorf("ioreg command failed: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "IOPlatformUUID") {
			// Line looks like: "IOPlatformUUID" = "XXXXXXXX-..."
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				id := strings.Trim(strings.TrimSpace(parts[1]), "\"")
				if id != "" {
					return id, nil
				}
			}
		}
	}
	return "", fmt.Errorf("IOPlatformUUID not found in ioreg output")
}

func readWindowsMachineID() (string, error) {
	out, err := exec.Command("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`,
		"/v", "MachineGuid").Output()
	if err != nil {
		return "", fmt.Errorf("reg query failed: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "MachineGuid") {
			// Line: "    MachineGuid    REG_SZ    xxxxxxxx-xxxx-..."
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				return fields[len(fields)-1], nil
			}
		}
	}
	return "", fmt.Errorf("MachineGuid not found in registry")
}

// uuidV5 generates a UUID v5 (SHA-1 based, deterministic) from a namespace UUID and a name.
func uuidV5(namespace [16]byte, name string) string {
	h := sha1.New()
	h.Write(namespace[:])
	h.Write([]byte(name))
	sum := h.Sum(nil)

	// Set version (5) and variant (RFC 4122).
	sum[6] = (sum[6] & 0x0f) | 0x50 // version 5
	sum[8] = (sum[8] & 0x3f) | 0x80 // variant RFC 4122

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
}
