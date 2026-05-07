package svcmanager

// ServiceStatus holds the status of a system service.
type ServiceStatus struct {
	Name        string `json:"name"`
	Active      string `json:"active"`
	Sub         string `json:"sub"`
	Description string `json:"description"`
	Enabled     string `json:"enabled"`
	PID         string `json:"pid,omitempty"`
}
