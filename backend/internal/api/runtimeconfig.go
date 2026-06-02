package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/skua-app/skua/internal/probe"
	"github.com/skua-app/skua/internal/runtimeconfig"
)

// runtimeConfigURLs is the URL triple used in three places of the
// runtime-config payload (effective / overlay / PUT body). Keeping a
// single type avoids drift between them.
type runtimeConfigURLs struct {
	FrigateURL   string `json:"frigate_url"`
	FrigateUIURL string `json:"frigate_ui_url"`
	Go2RTCURL    string `json:"go2rtc_url"`
}

// runtimeConfigLocked carries env-provenance flags. true means "this
// field is set via an environment variable" → the SPA renders it
// read-only and PUT strips it server-side.
type runtimeConfigLocked struct {
	FrigateURL   bool `json:"frigate_url"`
	FrigateUIURL bool `json:"frigate_ui_url"`
	Go2RTCURL    bool `json:"go2rtc_url"`
}

type runtimeConfigResponse struct {
	Effective runtimeConfigURLs   `json:"effective"`
	Overlay   runtimeConfigURLs   `json:"overlay"`
	Locked    runtimeConfigLocked `json:"locked"`
}

type runtimeConfigPutRequest struct {
	FrigateURL   string `json:"frigate_url"`
	FrigateUIURL string `json:"frigate_ui_url"`
	Go2RTCURL    string `json:"go2rtc_url"`
}

type runtimeConfigTestRequest struct {
	FrigateURL string `json:"frigate_url"`
	Go2RTCURL  string `json:"go2rtc_url"`
}

const runtimeConfigTestTimeout = 3 * time.Second

func (h *Handler) handleGetRuntimeConfig(w http.ResponseWriter, _ *http.Request) {
	h.writeRuntimeConfigResponse(w, http.StatusOK)
}

func (h *Handler) handlePutRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	if h.runtime.Store == nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "Runtime config store not configured.", nil)
		return
	}
	var req runtimeConfigPutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "Invalid request body.", err)
		return
	}

	values := runtimeconfig.Values{
		FrigateURL:   strings.TrimSpace(req.FrigateURL),
		FrigateUIURL: strings.TrimSpace(req.FrigateUIURL),
		Go2RTCURL:    strings.TrimSpace(req.Go2RTCURL),
	}

	// Drop env-locked fields server-side. The container env will win at
	// next boot regardless; storing them in the overlay would only
	// confuse a future operator reading config.yaml. Mirrors the
	// internal/setup wizard's save handler.
	current := h.runtime.Store.Get()
	if h.runtime.FrigateURLFromEnv {
		values.FrigateURL = current.FrigateURL
	}
	if h.runtime.FrigateUIURLFromEnv {
		values.FrigateUIURL = current.FrigateUIURL
	}
	if h.runtime.Go2RTCURLFromEnv {
		values.Go2RTCURL = current.Go2RTCURL
	}

	if values.FrigateURL == "" {
		writeError(w, h.logger, http.StatusBadRequest, "frigate_url_required", "Frigate URL is required.", nil)
		return
	}
	if err := probe.ValidateURL(values.FrigateURL); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "frigate_url_invalid", fmt.Sprintf("Frigate URL: %s", err), nil)
		return
	}
	if values.Go2RTCURL != "" {
		if err := probe.ValidateURL(values.Go2RTCURL); err != nil {
			writeError(w, h.logger, http.StatusBadRequest, "go2rtc_url_invalid", fmt.Sprintf("go2rtc URL: %s", err), nil)
			return
		}
	}
	if values.FrigateUIURL != "" {
		if err := probe.ValidateURL(values.FrigateUIURL); err != nil {
			writeError(w, h.logger, http.StatusBadRequest, "frigate_ui_url_invalid", fmt.Sprintf("Frigate UI URL: %s", err), nil)
			return
		}
	}

	if err := h.runtime.Store.Save(values); err != nil {
		if errors.Is(err, fs.ErrPermission) {
			writeError(w, h.logger, http.StatusInternalServerError, "data_not_writable",
				"Skua's data directory is not writable by the container user (uid/gid 65532). "+
					"Run chown -R 65532:65532 on the host data directory, or switch to a named Docker volume.", err)
			return
		}
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "Could not save runtime config.", err)
		return
	}

	h.writeRuntimeConfigResponse(w, http.StatusOK)
}

func (h *Handler) handleTestRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	var req runtimeConfigTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "Invalid request body.", err)
		return
	}
	report := probe.Report{
		Frigate: probe.Frigate(r.Context(), req.FrigateURL, runtimeConfigTestTimeout),
		Go2RTC:  probe.Go2RTC(r.Context(), req.Go2RTCURL, runtimeConfigTestTimeout),
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(report); err != nil {
		h.logger.Error("encode runtime-config test response", "error", err)
	}
}

func (h *Handler) handleRestartRuntimeConfig(w http.ResponseWriter, _ *http.Request) {
	if h.runtime.RequestRestart == nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "Restart not available.", nil)
		return
	}
	h.runtime.RequestRestart()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "restarting"})
}

func (h *Handler) writeRuntimeConfigResponse(w http.ResponseWriter, status int) {
	resp := runtimeConfigResponse{
		Effective: runtimeConfigURLs{
			FrigateURL:   h.runtime.FrigateURL,
			FrigateUIURL: h.runtime.FrigateUIURL,
			Go2RTCURL:    h.runtime.Go2RTCURL,
		},
		Locked: runtimeConfigLocked{
			FrigateURL:   h.runtime.FrigateURLFromEnv,
			FrigateUIURL: h.runtime.FrigateUIURLFromEnv,
			Go2RTCURL:    h.runtime.Go2RTCURLFromEnv,
		},
	}
	if h.runtime.Store != nil {
		v := h.runtime.Store.Get()
		resp.Overlay = runtimeConfigURLs{
			FrigateURL:   v.FrigateURL,
			FrigateUIURL: v.FrigateUIURL,
			Go2RTCURL:    v.Go2RTCURL,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("encode runtime-config response", "error", err)
	}
}
