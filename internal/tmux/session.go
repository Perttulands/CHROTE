package tmux

// Session represents a tmux session with metadata
type Session struct {
	Name     string `json:"name"`     // Full session name (e.g., "gt-gastown-jack")
	Windows  int    `json:"windows"`  // Number of tmux windows
	Attached bool   `json:"attached"` // Whether session is attached
	Group    string `json:"group"`    // Category (hq/main/gt-*/other)
}

// SessionsResponse is the API response for listing sessions
type SessionsResponse struct {
	Sessions  []Session            `json:"sessions"`
	Grouped   map[string][]Session `json:"grouped"`
	Timestamp string               `json:"timestamp"`
	Error     string               `json:"error,omitempty"`
}

// CreateSessionRequest is the request body for creating sessions
type CreateSessionRequest struct {
	Name string `json:"name,omitempty"`
}

// CreateSessionResponse is the response for session creation
type CreateSessionResponse struct {
	Success   bool   `json:"success"`
	Session   string `json:"session"`
	Timestamp string `json:"timestamp"`
	Error     string `json:"error,omitempty"`
}

// NukeResponse is the response for killing all sessions
type NukeResponse struct {
	Success   bool     `json:"success"`
	Killed    int      `json:"killed"`
	Sessions  []string `json:"sessions"`
	Timestamp string   `json:"timestamp"`
	Error     string   `json:"error,omitempty"`
}
