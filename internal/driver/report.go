package driver

// Report captures the effective WinDivert file/service state and any actions
// taken by EnsureWithReport.
type Report struct {
	ResolvedDir                  string `json:"resolved_dir,omitempty"`
	ResolvedSysPath              string `json:"resolved_sys_path,omitempty"`
	ServiceName                  string `json:"service_name,omitempty"`
	FilesPresent                 bool   `json:"files_present"`
	ServiceExists                bool   `json:"service_exists"`
	ServiceRunning               bool   `json:"service_running"`
	ServiceBinPath               string `json:"service_bin_path,omitempty"`
	ServiceBinPathExists         bool   `json:"service_bin_path_exists"`
	ServiceBinPathMatchesDesired bool   `json:"service_bin_path_matches_desired"`
	ServiceNeedsConfigRepair     bool   `json:"service_needs_config_repair"`
	ServiceCreated               bool   `json:"service_created"`
	ServiceReconfigured          bool   `json:"service_reconfigured"`
	ServiceStarted               bool   `json:"service_started"`
	CleanupWillStop              bool   `json:"cleanup_will_stop"`
	CleanupWillDelete            bool   `json:"cleanup_will_delete"`
}

// Issues returns install-health problems that should be surfaced by manual
// verification tooling.
func (r Report) Issues() []string {
	var issues []string
	if !r.FilesPresent {
		issues = append(issues, "WinDivert files are missing")
	}
	if !r.ServiceExists {
		issues = append(issues, "WinDivert service is missing")
		return issues
	}
	if r.ServiceBinPath == "" {
		issues = append(issues, "WinDivert service binary path is empty")
	}
	if !r.ServiceBinPathExists {
		issues = append(issues, "WinDivert service binary path does not exist")
	}
	if !r.ServiceBinPathMatchesDesired {
		issues = append(issues, "WinDivert service binary path does not match the expected driver path")
	}
	return issues
}

func (r Report) Healthy() bool {
	return len(r.Issues()) == 0
}
