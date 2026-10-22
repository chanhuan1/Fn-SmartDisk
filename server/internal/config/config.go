// Package config resolves runtime configuration for SmartDisk. All settings
// come from environment variables so the same binary can run in dev and in the
// fnOS app layout without recompilation.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Mode selects which process role the binary performs.
type Mode string

const (
	// ModeAll runs collector + web in a single process (used in dev and as
	// the default on-device layout where the panel reads SMART directly).
	ModeAll Mode = "all"
	// ModeCollector runs only the root collector daemon over a Unix socket.
	ModeCollector Mode = "collector"
	// ModeWeb runs only the web panel, proxying to the collector socket.
	ModeWeb Mode = "web"
)

// Config holds all runtime settings.
type Config struct {
	Mode Mode

	// Version is the app version, injected from main (build-time from
	// SmartDisk/manifest). Exposed via /api/health for the UI.
	Version string

	// HTTP listen address for development or split-process TCP mode.
	HTTPAddr string
	// Port is an explicit TCP port for development or split-process mode.
	// Zero selects an ephemeral loopback port.
	Port int

	// SocketPath is the Unix domain socket the collector listens on and the
	// web panel dials in split mode.
	SocketPath string

	// Gateway enables fnOS unified-gateway serving: the panel listens on a
	// Unix socket and all routes live under GatewayPrefix. Enabled when
	// GatewaySocket is set.
	GatewaySocket string
	GatewayPrefix string
	// GatewaySocketMode controls access to the unified-gateway Unix socket.
	GatewaySocketMode os.FileMode

	// WebRoot is the directory that contains the built frontend assets.
	WebRoot string

	// SmartctlPath and NvmePath point at the bundled (or system) binaries.
	SmartctlPath string
	NvmePath     string
	LsblkPath    string
	LsusbPath    string
	HdparmPath   string

	// DataDir persists runtime state (hibernate map). On fnOS this is the
	// app data dir (@appdata/<app>); in dev it falls back to a temp dir.
	DataDir string

	// MockDir, when set, makes the collector read canned command output from
	// files in this directory instead of invoking real binaries. Used in dev.
	MockDir string
}

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvFileMode(key string, def os.FileMode) os.FileMode {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(v, "0o"), 8, 32)
	if err != nil {
		return def
	}
	return os.FileMode(n).Perm()
}

// firstExisting returns the first path that exists, else def.
func firstExisting(def string, candidates ...string) string {
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return def
}

// appDir locates the fnOS app target dir (parent of bin/).
func appDir() string {
	if v := os.Getenv("TRIM_APPDEST"); v != "" {
		return v
	}
	// Dev fallback: assume <cwd> is server/, app assets are siblings.
	if wd, err := os.Getwd(); err == nil {
		return filepath.Dir(wd)
	}
	return "."
}

// Load builds a Config from the environment.
func Load() Config {
	dir := appDir()

	mode := Mode(strings.ToLower(getenv("SMARTDISK_MODE", string(ModeAll))))
	switch mode {
	case ModeAll, ModeCollector, ModeWeb:
	default:
		mode = ModeAll
	}

	port := getenvInt("SMARTDISK_PORT", 0)
	httpAddr := getenv("SMARTDISK_HTTP_ADDR", "")

	sock := getenv("SMARTDISK_SOCKET", "")
	if sock == "" {
		varDir := os.Getenv("TRIM_PKGVAR")
		if varDir == "" {
			varDir = os.TempDir()
		}
		sock = filepath.Join(varDir, "smartdisk-collector.sock")
	}

	// Unified gateway: fnOS forwards /app/{prefix} to this socket. The socket
	// must live in the app target dir for the gateway to reach it.
	gwSocket := getenv("SMARTDISK_GATEWAY_SOCKET", "")
	if gwSocket == "" && os.Getenv("TRIM_APPDEST") != "" {
		gwSocket = filepath.Join(dir, "app.sock")
	}
	gwPrefix := getenv("SMARTDISK_GATEWAY_PREFIX", "/app/smartdisk")
	// Gateway serving is only active when explicitly requested. Local
	// development can still opt into TCP with SMARTDISK_PORT or
	// SMARTDISK_HTTP_ADDR.
	if strings.ToLower(getenv("SMARTDISK_GATEWAY", "")) != "1" &&
		strings.ToLower(getenv("SMARTDISK_GATEWAY", "")) != "true" {
		gwSocket = ""
	}

	webRoot := getenv("SMARTDISK_WEB_ROOT", filepath.Join(dir, "www"))

	dataDir := getenv("SMARTDISK_DATA_DIR", "")
	if dataDir == "" {
		if v := os.Getenv("TRIM_APPDATA"); v != "" {
			dataDir = v
		} else {
			dataDir = filepath.Join(os.TempDir(), "smartdisk-data")
		}
	}

	binDir := filepath.Join(dir, "bin")
	smartctl := getenv("SMARTDISK_SMARTCTL",
		firstExisting("smartctl",
			filepath.Join(binDir, "smartctl"), "/usr/sbin/smartctl", "/usr/bin/smartctl"))
	nvme := getenv("SMARTDISK_NVME",
		firstExisting("nvme",
			filepath.Join(binDir, "nvme"), "/usr/sbin/nvme", "/usr/bin/nvme"))

	return Config{
		Mode:              mode,
		HTTPAddr:          httpAddr,
		Port:              port,
		SocketPath:        sock,
		GatewaySocket:     gwSocket,
		GatewayPrefix:     gwPrefix,
		GatewaySocketMode: getenvFileMode("SMARTDISK_GATEWAY_SOCKET_MODE", 0o600),
		WebRoot:           webRoot,
		SmartctlPath:      smartctl,
		NvmePath:          nvme,
		LsblkPath:         getenv("SMARTDISK_LSBLK", "lsblk"),
		LsusbPath:         getenv("SMARTDISK_LSUSB", "lsusb"),
		HdparmPath: getenv("SMARTDISK_HDPARM",
			firstExisting("hdparm", "/usr/sbin/hdparm", "/sbin/hdparm", "/usr/bin/hdparm")),
		DataDir: dataDir,
		MockDir: getenv("SMARTDISK_MOCK_DIR", ""),
	}
}

// GatewayEnabled reports whether the panel should serve over the unified
// gateway Unix socket instead of plain TCP.
func (c Config) GatewayEnabled() bool { return c.GatewaySocket != "" }

// ListenAddr resolves the explicit development TCP address. Packaged
// deployments use the fnOS unified gateway instead.
func (c Config) ListenAddr() string {
	if c.HTTPAddr != "" {
		return c.HTTPAddr
	}
	if c.Port <= 0 {
		return "127.0.0.1:0"
	}
	return "0.0.0.0:" + strconv.Itoa(c.Port)
}
