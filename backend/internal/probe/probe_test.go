package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFrigate_EmptyURLIsHardError(t *testing.T) {
	got := Frigate(context.Background(), "", time.Second)
	if got.OK {
		t.Errorf("OK = true, want false for empty URL")
	}
	if got.Skipped {
		t.Errorf("Skipped = true, want false (Frigate is required, not optional)")
	}
	if got.Error == "" {
		t.Errorf("Error is empty, want a diagnostic")
	}
	if !strings.Contains(got.Error, "Frigate URL") {
		t.Errorf("Error = %q, want message naming Frigate URL", got.Error)
	}
}

func TestGo2RTC_EmptyURLIsSkipped(t *testing.T) {
	got := Go2RTC(context.Background(), "", time.Second)
	if !got.Skipped {
		t.Errorf("Skipped = false, want true for empty go2rtc URL")
	}
	if got.OK {
		t.Errorf("OK = true, want false when skipped")
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty when skipped", got.Error)
	}
}

func TestFrigate_InvalidURLSurfacesValidationError(t *testing.T) {
	got := Frigate(context.Background(), "not a url", time.Second)
	if got.OK {
		t.Errorf("OK = true, want false for malformed URL")
	}
	if got.Error == "" {
		t.Errorf("Error is empty, want validation message")
	}
}

func TestGo2RTC_InvalidURLSurfacesValidationError(t *testing.T) {
	got := Go2RTC(context.Background(), "ftp://nope", time.Second)
	if got.OK {
		t.Errorf("OK = true, want false for non-http URL")
	}
	if got.Skipped {
		t.Errorf("Skipped = true, want false (URL was supplied)")
	}
	if got.Error == "" {
		t.Errorf("Error is empty, want validation message")
	}
}

func TestValidateURL(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"http://frigate:5000", false},
		{"https://frigate.example.com", false},
		{"ftp://frigate:5000", true},
		{"frigate:5000", true},
		{"", true},
		{"http://", true},
	}
	for _, c := range cases {
		err := ValidateURL(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateURL(%q) err = %v, wantErr = %v", c.in, err, c.wantErr)
		}
	}
}

func TestFrigate_HappyPathAgainstFakeUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/stats" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cpu_usages":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got := Frigate(context.Background(), srv.URL, time.Second)
	if !got.OK {
		t.Errorf("OK = false, want true; error = %q", got.Error)
	}
}

func TestGo2RTC_HappyPathAgainstFakeUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/streams" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"cam1":{}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	got := Go2RTC(context.Background(), srv.URL, time.Second)
	if !got.OK {
		t.Errorf("OK = false, want true; error = %q", got.Error)
	}
}
