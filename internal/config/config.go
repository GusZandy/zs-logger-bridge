// Package config handles loading/saving the bridge's small settings file:
// the logger URL, the per-logsheet ingest token, and the local UDP ports to
// listen on for N1MM and JTDX.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is persisted as JSON in the OS's per-user config directory (e.g.
// %AppData%\zs-logger-bridge on Windows, ~/Library/Application
// Support/zs-logger-bridge on macOS).
type Config struct {
	// ServerURL is the logger base URL, e.g. "https://logger.amatir.id".
	ServerURL string `json:"server_url"`
	// LogsheetID is the numeric id of the event/logsheet this bridge feeds.
	LogsheetID string `json:"logsheet_id"`
	// Token is the logsheet's ingest_token (from the "renew token" modal on
	// the logsheets page in the logger UI).
	Token string `json:"token"`
	// N1MMEnabled/N1MMPort control the N1MM XML listener. N1MM's default
	// "Contact info" broadcast port is commonly set to 12060 by operators,
	// but N1MM has no fixed default -- it must match Config -> Configure
	// Ports -> Broadcast Data in N1MM.
	N1MMEnabled bool `json:"n1mm_enabled"`
	N1MMPort    int  `json:"n1mm_port"`
	// JTDXEnabled/JTDXPort control the JTDX/WSJT-X UDP listener. 2237 is
	// the WSJT-X/JTDX default "UDP Server" port (Settings -> Reporting).
	JTDXEnabled bool `json:"jtdx_enabled"`
	JTDXPort    int  `json:"jtdx_port"`
}

// Default returns sane defaults for a first run.
func Default() Config {
	return Config{
		N1MMEnabled: true,
		N1MMPort:    12060,
		JTDXEnabled: true,
		JTDXPort:    2237,
	}
}

func path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "zs-logger-bridge")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// Load reads the config file, returning defaults if it doesn't exist yet.
func Load() (Config, error) {
	p, err := path()
	if err != nil {
		return Default(), err
	}

	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}

	cfg := Default()
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Default(), err
	}
	return cfg, nil
}

// Save writes the config file.
func (c Config) Save() error {
	p, err := path()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(p, data, 0o600)
}
