package httpapi

import (
	"net/http/httptest"
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
