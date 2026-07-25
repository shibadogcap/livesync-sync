//go:build notray

package tray

import "log/slog"

// noopManager is a no-op implementation for server/Docker builds.
type noopManager struct{}

// New creates a no-op tray manager (for server/Docker builds with CGO disabled).
func New(cfg Config) Manager {
	slog.Info("[Tray] Notray build: tray disabled")
	return &noopManager{}
}

func (m *noopManager) Run() error {
	return nil
}

func (m *noopManager) Stop() {}

func (m *noopManager) SetStatus(status string) {}

func (m *noopManager) SetSyncStatus(running bool) {}

func (m *noopManager) ShowNotification(title, message string) error {
	return nil
}
