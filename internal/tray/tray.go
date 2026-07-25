// Package tray provides cross-platform system tray support.
// The tray is only available in desktop builds (CGO_ENABLED=1).
// For server/Docker builds, use the `notray` build tag which provides a no-op stub.
package tray

// Manager is the interface for system tray operations.
type Manager interface {
	// Run starts the tray event loop. Blocks until Quit is called.
	Run() error

	// Stop cleans up the tray.
	Stop()

	// SetStatus updates the tray status display.
	SetStatus(status string)

	// SetSyncStatus updates the sync status indicator.
	SetSyncStatus(running bool)

	// ShowNotification shows a popup notification.
	ShowNotification(title, message string) error
}

// Config holds tray configuration.
type Config struct {
	Enable      bool
	Autostart   bool
	OnOpenVault func()   // Open vault folder
	OnSettings  func()   // Open settings UI
}
