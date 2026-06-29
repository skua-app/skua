package frigate

// CameraStats is the per-camera subset of GET /api/stats we care about.
type CameraStats struct {
	CameraFPS        float64 `json:"camera_fps"`
	DetectionEnabled bool    `json:"detection_enabled"`
	ProcessFPS       float64 `json:"process_fps"`
}

// StatsResponse is the subset of GET /api/stats we parse.
type StatsResponse struct {
	Cameras map[string]CameraStats `json:"cameras"`
	Service ServiceStats           `json:"service"`
}

// ServiceStats is the subset of stats.service we parse. Storage maps each
// mounted path Frigate reports to its usage figures.
type ServiceStats struct {
	Storage map[string]MountStat `json:"storage"`
}

// MountStat mirrors one stats.service.storage.<path> entry. Total/Used/Free
// are in MiB (mebibytes) as reported by Frigate.
type MountStat struct {
	Total     float64 `json:"total"`
	Used      float64 `json:"used"`
	Free      float64 `json:"free"`
	MountType string  `json:"mount_type"`
}

// ConfigResponse is the subset of GET /api/config we parse. Only the cameras
// subtree is decoded; all other Frigate config keys are ignored.
type ConfigResponse struct {
	Cameras map[string]CameraConfig `json:"cameras"`
}

// CameraConfig mirrors Frigate's cameras.<id> entry. Only the fields the BFF
// needs to derive a CameraSpec are decoded.
type CameraConfig struct {
	Name    string              `json:"name"`
	Enabled bool                `json:"enabled"`
	Live    FrigateLiveConfig   `json:"live"`
	FFmpeg  FrigateFFmpegConfig `json:"ffmpeg"`
}

// FrigateLiveConfig carries cameras.<id>.live. Streams maps the role key
// ("Main" / "Sub") to the go2rtc stream alias the BFF should use.
type FrigateLiveConfig struct {
	Streams map[string]string `json:"streams"`
}

// FrigateFFmpegConfig carries cameras.<id>.ffmpeg. Inputs is the only field we
// inspect — its first entry's Path is the fallback stream source when
// live.streams is absent.
type FrigateFFmpegConfig struct {
	Inputs []FrigateInput `json:"inputs"`
}

// FrigateInput mirrors a single cameras.<id>.ffmpeg.inputs[] entry. Only the
// path is decoded; roles/args are ignored.
type FrigateInput struct {
	Path string `json:"path"`
}
