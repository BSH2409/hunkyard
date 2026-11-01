package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BSH2409/hunkyard/internal/session"
)

func TestAttachmentGetRefusesOtherSites(t *testing.T) {
	s, _ := pinServer(t)
	file := savedShot(t, s)
	for _, tc := range []struct {
		site string
		want int
	}{
		{"", http.StatusOK},
		{"same-origin", http.StatusOK},
		{"none", http.StatusOK},
		{"same-site", http.StatusForbidden},
		{"cross-site", http.StatusForbidden},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/attachments/"+file, nil)
		if tc.site != "" {
			req.Header.Set("Sec-Fetch-Site", tc.site)
		}
		rec := httptest.NewRecorder()
		s.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("Sec-Fetch-Site %q = %d, want %d", tc.site, rec.Code, tc.want)
		}
		if tc.want == http.StatusOK {
			if got := rec.Header().Get("Cache-Control"); got != "private, no-store" {
				t.Errorf("Sec-Fetch-Site %q Cache-Control = %q", tc.site, got)
			}
			if rec.Body.Len() != len(pngBytes) {
				t.Errorf("Sec-Fetch-Site %q served %d bytes", tc.site, rec.Body.Len())
			}
		} else if rec.Body.Len() == len(pngBytes) {
			t.Errorf("Sec-Fetch-Site %q leaked the image", tc.site)
		}
	}
}

func TestDeletingPinRemovesItsScreenshot(t *testing.T) {
	s, _ := pinServer(t)
	file := savedShot(t, s)
	other := savedShot(t, s)
	rec := postPin(s, pinBody("Pay button", "#pay", shotJSON(file)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("pin = %d %s", rec.Code, rec.Body)
	}
	var c session.Comment
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatal(err)
	}
	del := httptest.NewRecorder()
	s.ServeHTTP(del, httptest.NewRequest(http.MethodDelete, "/api/comment/"+c.ID+"?path=%2F", nil))
	if del.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", del.Code, del.Body)
	}
	dir := session.ReviewPathsFor(s.reviewPath).Attachments
	if _, err := os.Stat(filepath.Join(dir, file)); !os.IsNotExist(err) {
		t.Errorf("pin screenshot still on disk after delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, other)); err != nil {
		t.Errorf("unrelated attachment removed: %v", err)
	}
}

func TestBaseBranchBodyLimit(t *testing.T) {
	s, _ := newTestServer(t)
	body := `{"base_branch":"` + strings.Repeat("a", 1<<20) + `"}`
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/base-branch", strings.NewReader(body)))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("1 MB base-branch body = %d, want 413", rec.Code)
	}
}
