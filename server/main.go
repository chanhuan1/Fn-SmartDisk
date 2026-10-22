// Command smartdisk is the single binary for the SmartDisk fnOS app. It can
// run the web panel, the root collector daemon, or both, selected by env.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"smartdisk/internal/collector"
	"smartdisk/internal/config"
	"smartdisk/internal/httpapi"
	"smartdisk/internal/model"
	"smartdisk/web"
)

// Version is the app version, injected at build time from SmartDisk/manifest
// via -ldflags "-X main.Version=...". Falls back to a dev marker.
var Version = "dev"

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("[smartdisk] ")

	// CLI mode: `smartdisk cli [list|get <id>|scan]` prints JSON to stdout and
	// exits. `scan` explicitly requests a full SMART read, which may wake disks.
	if len(os.Args) > 1 && os.Args[1] == "cli" {
		os.Exit(runCLI(os.Args[2:]))
	}

	cfg := config.Load()
	cfg.Version = Version

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	switch cfg.Mode {
	case config.ModeCollector:
		runCollector(ctx, cfg)
	case config.ModeWeb:
		runWeb(ctx, cfg)
	default:
		runAll(ctx, cfg)
	}
}

// runCLI prints cached-safe inventory by default. The explicit scan operation
// requests full SMART data and may wake a sleeping rotational disk.
func runCLI(args []string) int {
	cfg := config.Load()
	c := collector.New(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	op := "list"
	id := ""
	if len(args) > 0 {
		op = args[0]
	}
	if len(args) > 1 {
		id = args[1]
	}

	switch op {
	case "list":
		disks, err := c.List(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(disks)
		return 0
	case "scan":
		disks, err := c.Refresh(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(disks)
		return 0
	case "get":
		if id == "" {
			fmt.Fprintln(os.Stderr, "usage: smartdisk cli get <id>")
			return 2
		}
		d, err := c.Get(ctx, id)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 1
		}
		if d == nil {
			fmt.Fprintln(os.Stderr, "disk not found:", id)
			return 1
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(d)
		return 0
	default:
		fmt.Fprintln(os.Stderr, "usage: smartdisk cli [list|get <id>|scan]")
		return 2
	}
}

// localSource adapts the in-process Collector to the httpapi.DiskSource.
type localSource struct{ c *collector.Collector }

func (l localSource) ListDisks(ctx context.Context) ([]model.Disk, error) { return l.c.List(ctx) }
func (l localSource) OverviewJSON(ctx context.Context) ([]byte, bool) {
	return l.c.OverviewJSON(ctx)
}
func (l localSource) RefreshDisks(ctx context.Context) ([]model.Disk, error) {
	return l.c.Refresh(ctx)
}
func (l localSource) GetDisk(ctx context.Context, id string) (*model.Disk, error) {
	return l.c.Get(ctx, id)
}
func (l localSource) RefreshDisk(ctx context.Context, id string) (*model.Disk, error) {
	return l.c.RefreshDisk(ctx, id)
}
func (l localSource) Topology(ctx context.Context) ([]*model.StorageNode, error) {
	return l.c.Topology(ctx)
}
func (l localSource) SleepDisk(ctx context.Context, id string) (*collector.PowerResult, error) {
	return l.c.Sleep(ctx, id)
}
func (l localSource) OfflineDisk(ctx context.Context, id string, confirmRaid, confirmBusyIO bool) (*collector.PowerResult, error) {
	return l.c.Offline(ctx, id, confirmRaid, confirmBusyIO)
}
func (l localSource) WakeDisk(ctx context.Context, id string) (*collector.PowerResult, error) {
	return l.c.Wake(ctx, id)
}
func (l localSource) OfflineDiskStream(ctx context.Context, id string, confirmRaid, confirmBusyIO bool, steps collector.StepFunc) (*collector.PowerResult, error) {
	return l.c.OfflineWithSteps(ctx, id, confirmRaid, confirmBusyIO, steps)
}
func (l localSource) WakeDiskStream(ctx context.Context, id string, steps collector.StepFunc) (*collector.PowerResult, error) {
	return l.c.WakeWithSteps(ctx, id, steps)
}
func (l localSource) PowerLog(ctx context.Context, limit int) ([]model.PowerEvent, error) {
	return l.c.PowerEvents(limit), nil
}
func (l localSource) ClearPowerLog(ctx context.Context) error {
	return l.c.ClearPowerEvents()
}
func (l localSource) SetNote(ctx context.Context, id, note string) (*model.Disk, error) {
	return l.c.SetNote(ctx, id, note)
}

// ipcSource adapts the collector daemon client to httpapi.DiskSource.
type ipcSource struct{ cl collector.Client }

func (s ipcSource) ListDisks(ctx context.Context) ([]model.Disk, error) { return s.cl.List(ctx) }
func (s ipcSource) OverviewJSON(ctx context.Context) ([]byte, bool) {
	return nil, false // IPC payload is large; let List handle it normally
}
func (s ipcSource) RefreshDisks(ctx context.Context) ([]model.Disk, error) {
	return s.cl.Refresh(ctx)
}
func (s ipcSource) GetDisk(ctx context.Context, id string) (*model.Disk, error) {
	return s.cl.Get(ctx, id)
}
func (s ipcSource) RefreshDisk(ctx context.Context, id string) (*model.Disk, error) {
	return s.cl.RefreshDisk(ctx, id)
}
func (s ipcSource) Topology(ctx context.Context) ([]*model.StorageNode, error) {
	return s.cl.Topology(ctx)
}
func (s ipcSource) SleepDisk(ctx context.Context, id string) (*collector.PowerResult, error) {
	return s.cl.SleepDisk(ctx, id)
}
func (s ipcSource) OfflineDisk(ctx context.Context, id string, confirmRaid, confirmBusyIO bool) (*collector.PowerResult, error) {
	return s.cl.OfflineDisk(ctx, id, confirmRaid, confirmBusyIO)
}
func (s ipcSource) WakeDisk(ctx context.Context, id string) (*collector.PowerResult, error) {
	return s.cl.WakeDisk(ctx, id)
}

func (s ipcSource) OfflineDiskStream(ctx context.Context, id string, confirmRaid, confirmBusyIO bool, steps collector.StepFunc) (*collector.PowerResult, error) {
	return s.cl.OfflineDiskStream(ctx, id, confirmRaid, confirmBusyIO, steps)
}
func (s ipcSource) WakeDiskStream(ctx context.Context, id string, steps collector.StepFunc) (*collector.PowerResult, error) {
	return s.cl.WakeDiskStream(ctx, id, steps)
}
func (s ipcSource) PowerLog(ctx context.Context, limit int) ([]model.PowerEvent, error) {
	return s.cl.PowerLog(ctx, limit)
}
func (s ipcSource) ClearPowerLog(ctx context.Context) error {
	return s.cl.ClearPowerLog(ctx)
}
func (s ipcSource) SetNote(ctx context.Context, id, note string) (*model.Disk, error) {
	return s.cl.SetNote(ctx, id, note)
}

// staticFS returns the embedded web assets, or nil to serve from disk.
func staticFS() http.FileSystem {
	sub, err := fs.Sub(web.Assets, "dist")
	if err != nil || sub == nil {
		return nil
	}
	entries, err := fs.ReadDir(sub, ".")
	if err != nil || len(entries) == 0 {
		return nil
	}
	return http.FS(sub)
}

func runCollector(ctx context.Context, cfg config.Config) {
	c := collector.New(cfg)
	log.Printf("collector daemon on unix://%s", cfg.SocketPath)
	if err := collector.ServeDaemon(ctx, c, cfg.SocketPath); err != nil {
		log.Fatalf("collector: %v", err)
	}
}

func runWeb(ctx context.Context, cfg config.Config) {
	src := ipcSource{cl: collector.Client{SockPath: cfg.SocketPath}}
	serve(ctx, cfg, src)
}

func runAll(ctx context.Context, cfg config.Config) {
	src := localSource{c: collector.New(cfg)}
	serve(ctx, cfg, src)
}

func serve(ctx context.Context, cfg config.Config, src httpapi.DiskSource) {
	srv := &http.Server{
		Handler:           httpapi.New(cfg, src, staticFS(), cfg.MockDir != "").Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	if cfg.GatewayEnabled() {
		// Unified gateway: fnOS forwards /app/{prefix} to this Unix socket.
		_ = os.Remove(cfg.GatewaySocket)
		ln, err := net.Listen("unix", cfg.GatewaySocket)
		if err != nil {
			log.Fatalf("gateway listen: %v", err)
		}
		mode := cfg.GatewaySocketMode
		if mode == 0 {
			mode = 0o600
		}
		if err := os.Chmod(cfg.GatewaySocket, mode); err != nil {
			log.Fatalf("gateway socket permissions: %v", err)
		}
		log.Printf("web panel on gateway unix://%s prefix=%s (mode=%s mock=%v)",
			cfg.GatewaySocket, cfg.GatewayPrefix, cfg.Mode, cfg.MockDir != "")
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http(gateway): %v", err)
		}
		return
	}

	addr := cfg.ListenAddr()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("http listen: %v", err)
	}
	log.Printf("web panel on http://%s (mode=%s mock=%v)", ln.Addr(), cfg.Mode, cfg.MockDir != "")
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http: %v", err)
	}
}
