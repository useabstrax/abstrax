package web

// TestResult holds the result of a web server configuration test.
type TestResult struct {
	OK      bool   `json:"ok"`
	Output  string `json:"output"`
	Backend string `json:"backend"`
}
