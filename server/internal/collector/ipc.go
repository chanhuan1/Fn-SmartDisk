package collector

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"time"

	"smartdisk/internal/model"
)

// request is the IPC envelope from web panel to collector daemon.
type request struct {
	Op            string `json:"op"` // "list"|"refresh"|"get"|"topology"|"sleep"|"offline"|"wake"|"powerlog"
	ID            string `json:"id,omitempty"`
	ConfirmRaid   bool   `json:"confirmRaid,omitempty"`
	ConfirmBusyIO bool   `json:"confirmBusyIO,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	Stream        bool   `json:"stream,omitempty"`
	Note          string `json:"note,omitempty"`
}

type response struct {
	Type    string               `json:"type,omitempty"` // "step" or terminal "result"
	Step    string               `json:"step,omitempty"`
	Detail  string               `json:"detail,omitempty"`
	OK      bool                 `json:"ok"`
	Error   string               `json:"error,omitempty"`
	ErrKind string               `json:"errKind,omitempty"`
	Disks   []model.Disk         `json:"disks,omitempty"`
	Disk    *model.Disk          `json:"disk,omitempty"`
	Tree    []*model.StorageNode `json:"tree,omitempty"`
	Power   *PowerResult         `json:"power,omitempty"`
	Log     []model.PowerEvent   `json:"log,omitempty"`
}

// ServeDaemon listens on a Unix socket and serves SMART data to the web
// panel. Only local processes can connect; the socket is created with 0600
// so only the same user (root / the panel via group) can reach it.
func ServeDaemon(ctx context.Context, c *Collector, sockPath string) error {
	_ = os.Remove(sockPath)
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return err
	}
	// Restrict access: owner read/write only.
	_ = os.Chmod(sockPath, 0o600)

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go handleConn(c, conn)
	}
}

func handleConn(c *Collector, conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(65 * time.Second))
	var req request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		return
	}
	timeout := 60 * time.Second
	if req.Op == "powerlog" {
		timeout = 10 * time.Second
	}
	_ = conn.SetDeadline(time.Now().Add(timeout + 5*time.Second))
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var resp response
	enc := json.NewEncoder(conn)
	if req.Stream && (req.Op == "offline" || req.Op == "wake") {
		steps := func(step, detail string) {
			_ = enc.Encode(response{Type: "step", Step: step, Detail: detail})
		}
		var power *PowerResult
		var err error
		if req.Op == "offline" {
			power, err = c.OfflineWithSteps(ctx, req.ID, req.ConfirmRaid, req.ConfirmBusyIO, steps)
		} else {
			power, err = c.WakeWithSteps(ctx, req.ID, steps)
		}
		if err != nil {
			resp = response{Type: "result", OK: false, Error: err.Error()}
		} else {
			resp = response{Type: "result", OK: true, Power: power}
		}
		_ = enc.Encode(resp)
		return
	}

	switch req.Op {
	case "list":
		disks, err := c.List(ctx)
		if err != nil {
			resp = response{OK: false, Error: err.Error(), ErrKind: errKind(err)}
		} else {
			resp = response{OK: true, Disks: disks}
		}
	case "refresh":
		disks, err := c.Refresh(ctx)
		if err != nil {
			resp = response{OK: false, Error: err.Error(), ErrKind: errKind(err)}
		} else {
			resp = response{OK: true, Disks: disks}
		}
	case "get":
		d, err := c.Get(ctx, req.ID)
		if err != nil {
			resp = response{OK: false, Error: err.Error()}
		} else if d == nil {
			resp = response{OK: false, Error: "not found"}
		} else {
			resp = response{OK: true, Disk: d}
		}
	case "refreshdisk":
		d, err := c.RefreshDisk(ctx, req.ID)
		if err != nil {
			resp = response{OK: false, Error: err.Error(), ErrKind: errKind(err)}
		} else {
			resp = response{OK: true, Disk: d}
		}
	case "topology":
		tree, err := c.Topology(ctx)
		if err != nil {
			resp = response{OK: false, Error: err.Error()}
		} else {
			resp = response{OK: true, Tree: tree}
		}
	case "sleep", "offline", "wake":
		var res *PowerResult
		var err error
		switch req.Op {
		case "sleep":
			res, err = c.Sleep(ctx, req.ID)
		case "offline":
			res, err = c.Offline(ctx, req.ID, req.ConfirmRaid, req.ConfirmBusyIO)
		case "wake":
			res, err = c.Wake(ctx, req.ID)
		}
		if err != nil {
			resp = response{OK: false, Error: err.Error(), ErrKind: errKind(err)}
		} else {
			resp = response{OK: true, Power: res}
		}
	case "powerlog":
		resp = response{OK: true, Log: c.PowerEvents(req.Limit)}
	case "clearpowerlog":
		c.ClearPowerEvents()
		resp = response{OK: true}
	case "setnote":
		d, err := c.SetNote(ctx, req.ID, req.Note)
		if err != nil {
			resp = response{OK: false, Error: err.Error(), ErrKind: errKind(err)}
		} else if d == nil {
			resp = response{OK: false, Error: "not found"}
		} else {
			resp = response{OK: true, Disk: d}
		}
	default:
		resp = response{OK: false, Error: "unknown op"}
	}
	resp.Type = "result"
	_ = enc.Encode(resp)
}

// errKind maps collector sentinel errors to a wire-safe token. The client
// rebuilds the sentinel so httpapi keeps its semantic HTTP status mapping
// (403/409/400) in split web/collector mode.
func errKind(err error) string {
	switch {
	case errors.Is(err, ErrSystemDisk):
		return "system_disk"
	case errors.Is(err, ErrNotHDD):
		return "not_hdd"
	case errors.Is(err, ErrNeedRaidConfirm):
		return "need_raid_confirm"
	case errors.Is(err, ErrDiskBusy):
		return "disk_busy"
	case errors.Is(err, ErrDiskMissing):
		return "disk_missing"
	case errors.Is(err, ErrNotOffline):
		return "not_offline"
	default:
		return ""
	}
}

// sentinelForKind rebuilds the collector sentinel from a wire token.
func sentinelForKind(kind string) error {
	switch kind {
	case "system_disk":
		return ErrSystemDisk
	case "not_hdd":
		return ErrNotHDD
	case "need_raid_confirm":
		return ErrNeedRaidConfirm
	case "disk_busy":
		return ErrDiskBusy
	case "disk_missing":
		return ErrDiskMissing
	case "not_offline":
		return ErrNotOffline
	default:
		return nil
	}
}

// clientError reconstructs a sentinel when the response carries one; plain
// text otherwise preserves the daemon's message for display.
func clientError(resp *response) error {
	if kind := sentinelForKind(resp.ErrKind); kind != nil {
		return kind
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return errors.New("collector request failed")
}

// Client dials the collector daemon over the Unix socket.
type Client struct{ SockPath string }

func (cl Client) callContext(ctx context.Context, req request) (*response, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", cl.SockPath)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	timeout := 65 * time.Second
	if req.Op == "powerlog" {
		timeout = 15 * time.Second
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}
	var resp response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (cl Client) callStream(ctx context.Context, req request, steps StepFunc) (*response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req.Stream = true
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "unix", cl.SockPath)
	if err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	defer close(done)
	defer conn.Close()
	deadline := time.Now().Add(65 * time.Second)
	if d, ok := ctx.Deadline(); ok {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return nil, err
	}
	return readStream(json.NewDecoder(conn), steps)
}

func readStream(dec *json.Decoder, steps StepFunc) (*response, error) {
	for {
		var resp response
		if err := dec.Decode(&resp); err != nil {
			return nil, err
		}
		switch strings.ToLower(resp.Type) {
		case "step":
			if steps != nil {
				steps(resp.Step, resp.Detail)
			}
		case "result", "":
			return &resp, nil
		default:
			return nil, errors.New("unknown collector IPC frame type")
		}
	}
}

// List returns all disks via the daemon.
func (cl Client) List(ctx context.Context) ([]model.Disk, error) {
	resp, err := cl.callContext(ctx, request{Op: "list"})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Disks, nil
}

// Refresh forces a fresh collection via the daemon.
func (cl Client) Refresh(ctx context.Context) ([]model.Disk, error) {
	resp, err := cl.callContext(ctx, request{Op: "refresh"})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Disks, nil
}

// Get returns one disk via the daemon.
func (cl Client) Get(ctx context.Context, id string) (*model.Disk, error) {
	resp, err := cl.callContext(ctx, request{Op: "get", ID: id})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Disk, nil
}

// RefreshDisk re-collects one disk via the daemon.
func (cl Client) RefreshDisk(ctx context.Context, id string) (*model.Disk, error) {
	resp, err := cl.callContext(ctx, request{Op: "refreshdisk", ID: id})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Disk, nil
}

// Topology returns the storage tree via the daemon.
func (cl Client) Topology(ctx context.Context) ([]*model.StorageNode, error) {
	resp, err := cl.callContext(ctx, request{Op: "topology"})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Tree, nil
}

// power sends a power-control op to the daemon and returns the result.
func (cl Client) power(ctx context.Context, op, id string, confirmRaid, confirmBusyIO bool) (*PowerResult, error) {
	resp, err := cl.callContext(ctx, request{Op: op, ID: id, ConfirmRaid: confirmRaid, ConfirmBusyIO: confirmBusyIO})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Power, nil
}

// SleepDisk spins a disk down via the daemon.
func (cl Client) SleepDisk(ctx context.Context, id string) (*PowerResult, error) {
	return cl.power(ctx, "sleep", id, false, false)
}

// OfflineDisk deep-hibernates a disk via the daemon.
func (cl Client) OfflineDisk(ctx context.Context, id string, confirmRaid, confirmBusyIO bool) (*PowerResult, error) {
	return cl.power(ctx, "offline", id, confirmRaid, confirmBusyIO)
}

// WakeDisk brings a deep-offlined disk back via the daemon.
func (cl Client) WakeDisk(ctx context.Context, id string) (*PowerResult, error) {
	return cl.power(ctx, "wake", id, false, false)
}

// PowerLog returns the power-control event log via the daemon.
func (cl Client) PowerLog(ctx context.Context, limit int) ([]model.PowerEvent, error) {
	resp, err := cl.callContext(ctx, request{Op: "powerlog", Limit: limit})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Log, nil
}

// ClearPowerLog wipes the persisted event log via the daemon.
func (cl Client) ClearPowerLog(ctx context.Context) error {
	resp, err := cl.callContext(ctx, request{Op: "clearpowerlog"})
	if err != nil {
		return err
	}
	if !resp.OK {
		return clientError(resp)
	}
	return nil
}

// SetNote persists a user note for a disk via the daemon.
func (cl Client) SetNote(ctx context.Context, id, note string) (*model.Disk, error) {
	resp, err := cl.callContext(ctx, request{Op: "setnote", ID: id, Note: note})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Disk, nil
}

// OfflineDiskStream streams operation steps over the collector socket.
func (cl Client) OfflineDiskStream(ctx context.Context, id string, confirmRaid, confirmBusyIO bool, steps StepFunc) (*PowerResult, error) {
	resp, err := cl.callStream(ctx, request{Op: "offline", ID: id, ConfirmRaid: confirmRaid, ConfirmBusyIO: confirmBusyIO}, steps)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Power, nil
}

// WakeDiskStream streams operation steps over the collector socket.
func (cl Client) WakeDiskStream(ctx context.Context, id string, steps StepFunc) (*PowerResult, error) {
	resp, err := cl.callStream(ctx, request{Op: "wake", ID: id}, steps)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, clientError(resp)
	}
	return resp.Power, nil
}
