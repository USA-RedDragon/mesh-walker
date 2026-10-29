package apimodels

// LQM is the lqm block of a sysinfo response, copied from mesh-manager's
// internal/services/lqm package so the encoded output stays the same.
type LQM struct {
	Enabled bool    `json:"enabled"`
	Info    LQMInfo `json:"info,omitempty"`
}

type LQMInfo struct {
	Trackers        any   `json:"trackers,omitempty"`
	Start           int64 `json:"start,omitempty"`
	Now             int64 `json:"now,omitempty"`
	Distance        any   `json:"distance,omitempty"`
	TotalRouteCount int64 `json:"total_route_count,omitempty"`
}
