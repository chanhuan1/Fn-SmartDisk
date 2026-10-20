package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRefreshRejectsGET(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/disks/refresh", nil)
	s.handleRefresh(rec, req)
	if rec.Code != 405 {
		t.Fatalf("GET /api/disks/refresh status=%d, want 405", rec.Code)
	}
}

func TestReadEndpointsRejectWrongMethods(t *testing.T) {
	s := &Server{}
	tests := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
		path string
	}{
		{"health", s.handleHealth, "/api/health"},
		{"topology", s.handleTopology, "/api/topology"},
		{"power log", s.handlePowerLog, "/api/power/log"},
		{"disk detail", s.handleDiskByID, "/api/disks/sda"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", tt.path, nil)
			tt.call(rec, req)
			if rec.Code != 405 {
				t.Fatalf("status=%d want 405", rec.Code)
			}
		})
	}
}

func TestNoteBodyLimit(t *testing.T) {
	s := &Server{}
	body := strings.NewReader(`{"note":"` + strings.Repeat("x", 5000) + `"}`)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/disks/sda/note", body)
	s.handleSetNote(rec, req, "sda")
	if rec.Code != 400 {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestDiskUnknownNestedResourceRejected(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/disks/sda/a/b", nil)
	s.handleDiskByID(rec, req)
	if rec.Code != 404 {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}
