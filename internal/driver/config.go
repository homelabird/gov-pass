package driver

// Config controls WinDivert driver installation and lifecycle.
type Config struct {
	Dir           string
	SysName       string
	ServiceName   string
	AutoInstall   bool
	AutoUninstall bool
	AutoStop      bool
}
