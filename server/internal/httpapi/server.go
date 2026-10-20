// Package httpapi exposes the read-only disk/SMART REST API and serves the
// embedded web panel.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"smartdisk/internal/collector"
	"smartdisk/internal/config"
	"smartdisk/internal/model"
)

// DiskSource abstracts where disk data comes from: the in-process collector
// (ModeAll) or the root collector daemon over IPC (ModeWeb).
type DiskSource interface {
	ListDisks(ctx context.Context) ([]model.Disk, error)
	// OverviewJSON returns the pre-serialized /api/disks payload when the
	// source has one available. Returning false falls back to ListDisks.
	OverviewJSON(ctx context.Context) ([]byte, bool)
	RefreshDisks(ctx context.Context) ([]model.Disk, error)
	GetDisk(ctx context.Context, id string) (*model.Disk, error)
	// RefreshDisk re-collects ONLY one disk (does not wake others).
	RefreshDisk(ctx context.Context, id string) (*model.Disk, error)
	Topology(ctx context.Context) ([]*model.StorageNode, error)
	// Power control (mutating; see collector/power.go).
	SleepDisk(ctx context.Context, id string) (*collector.PowerResult, error)
	OfflineDisk(ctx context.Context, id string, confirmRaid, confirmBusyIO bool) (*collector.PowerResult, error)
	WakeDisk(ctx context.Context, id string) (*collector.PowerResult, error)
	// Streaming variants emit each step via the callback (SSE).
	OfflineDiskStream(ctx context.Context, id string, confirmRaid, confirmBusyIO bool, steps collector.StepFunc) (*collector.PowerResult, error)
	WakeDiskStream(ctx context.Context, id string, steps collector.StepFunc) (*collector.PowerResult, error)
	// PowerLog returns the power-control event log (newest first).
	PowerLog(ctx context.Context, limit int) ([]model.PowerEvent, error)
	// ClearPowerLog wipes the persisted event log.
	ClearPowerLog(ctx context.Context) error
	// SetNote persists a user-maintained note for a disk ("" clears it).
	SetNote(ctx context.Context, id, note string) (*model.Disk, error)
}

// Server is the panel HTTP server.
type Server struct {
	cfg    config.Config
	src    DiskSource
	mock   bool
	mux    *http.ServeMux
	prefix string // route prefix, "" or "/app/smartdisk" under the gateway
}

// New builds a Server. staticFS may be nil, in which case WebRoot is served
// from disk.
func New(cfg config.Config, src DiskSource, staticFS http.FileSystem, mock bool) *Server {
	prefix := ""
	if cfg.GatewayEnabled() {
		prefix = strings.TrimSuffix(cfg.GatewayPrefix, "/")
	}
	s := &Server{cfg: cfg, src: src, mock: mock, mux: http.NewServeMux(), prefix: prefix}
	s.routes(staticFS)
	return s
}

// idRe matches kernel block-device names (sda, nvme0n1). Keep it in sync
// with collector's powerIDRe so a valid API id cannot become an unsafe sysfs
// path suffix.
var idRe = regexp.MustCompile(`^[a-z0-9]+$`)

// p joins a route onto the gateway prefix.
func (s *Server) p(path string) string { return s.prefix + path }

func (s *Server) routes(staticFS http.FileSystem) {
	// The fnOS gateway may forward the path with the prefix stripped or
	// intact depending on version; register handlers for BOTH forms so either
	// behaviour works. In non-gateway mode prefix is "" so these coincide.
	prefixes := []string{s.prefix}
	if s.prefix != "" {
		prefixes = append(prefixes, "")
	}
	for _, px := range prefixes {
		s.mux.HandleFunc(px+"/api/health", s.handleHealth)
		s.mux.HandleFunc(px+"/api/disks", s.handleDisks)
		s.mux.HandleFunc(px+"/api/disks/refresh", s.handleRefresh)
		s.mux.HandleFunc(px+"/api/disks/", s.handleDiskByID)
		s.mux.HandleFunc(px+"/api/topology", s.handleTopology)
		s.mux.HandleFunc(px+"/api/power/log", s.handlePowerLog)
		s.mux.HandleFunc(px+"/api/power/log/clear", s.handleClearPowerLog)
	}

	var fs http.Handler
	if staticFS != nil {
		fs = http.FileServer(staticFS)
	} else {
		fs = http.FileServer(http.Dir(s.cfg.WebRoot))
	}

	// SPA fallback for the root path (both prefixed and bare).
	roots := []string{s.p("/")}
	if s.prefix != "" {
		roots = append(roots, "/")
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Normalize: strip the gateway prefix (if present) so asset lookup is
		// relative to the web root regardless of gateway behaviour.
		rel := r.URL.Path
		if s.prefix != "" && strings.HasPrefix(rel, s.prefix) {
			rel = strings.TrimPrefix(rel, s.prefix)
		}
		if strings.HasPrefix(rel, "/api/") {
			http.NotFound(w, r)
			return
		}
		p := strings.TrimPrefix(rel, "/")
		if p == "" {
			r2 := new(http.Request)
			*r2 = *r
			r2.URL.Path = "/"
			fs.ServeHTTP(w, r2)
			return
		}
		if staticFS != nil {
			if f, err := staticFS.Open("/" + p); err == nil {
				_ = f.Close()
				r3 := new(http.Request)
				*r3 = *r
				r3.URL.Path = rel
				fs.ServeHTTP(w, r3)
				return
			}
			r2 := new(http.Request)
			*r2 = *r
			r2.URL.Path = "/"
			fs.ServeHTTP(w, r2)
			return
		}
		r4 := new(http.Request)
		*r4 = *r
		r4.URL.Path = rel
		fs.ServeHTTP(w, r4)
	})
	for _, rt := range roots {
		s.mux.Handle(rt, handler)
	}
}

// Handler returns the root handler with security headers applied.
func (s *Server) Handler() http.Handler {
	return s.secureHeaders(s.mux)
}

func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		// The gateway terminates at the system origin; framing is governed by
		// fnOS itself, so we only restrict when serving on a bare port.
		if !s.cfg.GatewayEnabled() {
			h.Set("X-Frame-Options", "SAMEORIGIN")
		}
		p := r.URL.Path
		if s.prefix != "" {
			p = strings.TrimPrefix(p, s.prefix)
		}
		if strings.HasPrefix(p, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(true)
	_ = enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "time": time.Now().Unix(), "version": s.cfg.Version})
}

// handleRefreshDisk serves POST /api/disks/{id}/refresh: re-collect only this
// disk (does not wake others) and return its fresh detail.
func (s *Server) handleRefreshDisk(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	d, err := s.src.RefreshDisk(ctx, id)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "刷新失败: "+err.Error())
		return
	}
	if d == nil {
		writeErr(w, http.StatusNotFound, "未找到磁盘 "+id)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handlePowerLog serves GET /api/power/log?limit=N (default 100, max 500).
func (s *Server) handlePowerLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	limit := 100
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	evts, err := s.src.PowerLog(ctx, limit)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "读取日志失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": evts, "count": len(evts)})
}

// handleClearPowerLog serves POST /api/power/log/clear.
func (s *Server) handleClearPowerLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.src.ClearPowerLog(ctx); err != nil {
		writeErr(w, http.StatusBadGateway, "清空日志失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSetNote serves POST /api/disks/{id}/note with {"note": "..."}.
func (s *Server) handleSetNote(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var body struct {
		Note string `json:"note"`
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil && err.Error() != "EOF" {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}
	if len(body.Note) > 200 {
		writeErr(w, http.StatusBadRequest, "备注过长（最多 200 字符）")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	d, err := s.src.SetNote(ctx, id, body.Note)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "保存备注失败: "+err.Error())
		return
	}
	if d == nil {
		writeErr(w, http.StatusNotFound, "未找到磁盘 "+id)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDisks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if data, ok := s.src.OverviewJSON(ctx); ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}
	disks, err := s.src.ListDisks(ctx)
	s.writeOverview(w, disks, err)
}

// handleRefresh forces a fresh collection (user clicked "refresh"), which may
// wake standby drives. Returns the new overview.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	disks, err := s.src.RefreshDisks(ctx)
	s.writeOverview(w, disks, err)
}

// writeOverview builds the /api/disks response from a disks+err pair.
func (s *Server) writeOverview(w http.ResponseWriter, disks []model.Disk, err error) {
	if err != nil {
		writeJSON(w, http.StatusOK, model.Overview{Error: "采集失败: " + err.Error(), Generated: time.Now().Unix(), Mock: s.mock})
		return
	}
	sums := make([]model.Summary, 0, len(disks))
	for i := range disks {
		sums = append(sums, disks[i].ToSummary())
	}
	writeJSON(w, http.StatusOK, model.Overview{
		Disks:     sums,
		Count:     len(sums),
		Generated: collectedAt(disks),
		Mock:      s.mock,
	})
}

// collectedAt reports when the disk data was actually gathered, taken from the
// newest disk's CollectedAt stamp. The cache serves repeat page views without
// re-collecting, so using time.Now() here would wrongly show "now" (the page
// open time) as the collection time. Falls back to now when no stamp exists.
func collectedAt(disks []model.Disk) int64 {
	var newest int64
	for i := range disks {
		if disks[i].CollectedAt > newest {
			newest = disks[i].CollectedAt
		}
	}
	if newest == 0 {
		return time.Now().Unix()
	}
	return newest
}

func (s *Server) handleTopology(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "use GET")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	tree, err := s.src.Topology(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "采集失败: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tree":      tree,
		"generated": time.Now().Unix(),
	})
}

func (s *Server) handleDiskByID(w http.ResponseWriter, r *http.Request) {
	rest := r.URL.Path
	if s.prefix != "" {
		rest = strings.TrimPrefix(rest, s.prefix)
	}
	rest = strings.TrimPrefix(rest, "/api/disks/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) == 0 || parts[0] == "" || !idRe.MatchString(parts[0]) {
		writeErr(w, http.StatusBadRequest, "invalid disk id")
		return
	}
	id := parts[0]

	// Per-disk refresh sub-resource: POST /api/disks/{id}/refresh
	if len(parts) == 2 && parts[1] == "refresh" {
		s.handleRefreshDisk(w, r, id)
		return
	}
	// Per-disk note sub-resource: POST /api/disks/{id}/note
	if len(parts) == 2 && parts[1] == "note" {
		s.handleSetNote(w, r, id)
		return
	}
	// Power action sub-resource: POST /api/disks/{id}/{sleep,offline,wake}
	if len(parts) == 2 {
		s.handlePowerAction(w, r, id, parts[1])
		return
	}
	if len(parts) != 1 {
		writeErr(w, http.StatusNotFound, "unknown disk sub-resource")
		return
	}
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "use GET")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	d, err := s.src.GetDisk(ctx, id)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "采集失败: "+err.Error())
		return
	}
	if d == nil {
		writeErr(w, http.StatusNotFound, "未找到磁盘 "+id)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handlePowerAction executes POST /api/disks/{id}/{sleep,offline,wake}. The
// offline action accepts optional acknowledgements {"confirmRaid":true,
// "confirmBusyIO":true}.
func (s *Server) handlePowerAction(w http.ResponseWriter, r *http.Request, id, action string) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "use POST")
		return
	}
	var body struct {
		ConfirmRaid   bool `json:"confirmRaid"`
		ConfirmBusyIO bool `json:"confirmBusyIO"`
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil && err.Error() != "EOF" {
			writeErr(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	// SSE streaming mode: client asked for live step output.
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		s.handlePowerStream(w, r, ctx, id, action, body.ConfirmRaid, body.ConfirmBusyIO)
		return
	}

	var res *collector.PowerResult
	var err error
	switch action {
	case "sleep":
		res, err = s.src.SleepDisk(ctx, id)
	case "offline":
		res, err = s.src.OfflineDisk(ctx, id, body.ConfirmRaid, body.ConfirmBusyIO)
	case "wake":
		res, err = s.src.WakeDisk(ctx, id)
	default:
		writeErr(w, http.StatusNotFound, "unknown action "+action)
		return
	}
	if err != nil {
		code := http.StatusBadGateway
		switch {
		case errors.Is(err, collector.ErrNeedRaidConfirm),
			errors.Is(err, collector.ErrDiskBusy):
			code = http.StatusConflict
		case errors.Is(err, collector.ErrSystemDisk),
			errors.Is(err, collector.ErrNotHDD):
			code = http.StatusForbidden
		case errors.Is(err, collector.ErrDiskMissing),
			errors.Is(err, collector.ErrNotOffline):
			code = http.StatusBadRequest
		}
		writeJSON(w, code, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handlePowerStream runs an offline/wake op and streams each step as an SSE
// event: `data: {"step":..,"detail":..}` lines, then a final
// `data: {"done":true,"ok":..,"message":..}`.
func (s *Server) handlePowerStream(w http.ResponseWriter, r *http.Request, ctx context.Context, id, action string, confirmRaid, confirmBusyIO bool) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeErr(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	send := func(v any) {
		b, err := json.Marshal(v)
		if err != nil {
			return
		}
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(b)
		_, _ = w.Write([]byte("\n\n"))
		fl.Flush()
	}
	steps := func(step, detail string) {
		send(map[string]string{"step": step, "detail": detail})
	}

	var res *collector.PowerResult
	var err error
	switch action {
	case "offline":
		res, err = s.src.OfflineDiskStream(ctx, id, confirmRaid, confirmBusyIO, steps)
	case "wake":
		res, err = s.src.WakeDiskStream(ctx, id, steps)
	default:
		send(map[string]any{"done": true, "ok": false, "error": "该操作不支持实时输出"})
		return
	}
	if err != nil {
		send(map[string]any{"done": true, "ok": false, "error": err.Error()})
		return
	}
	send(map[string]any{"done": true, "ok": true, "message": res.Message})
}
