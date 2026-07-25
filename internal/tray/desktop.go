//go:build !notray

package tray

import (
	"log/slog"

	"github.com/getlantern/systray"
)

// desktopManager implements the tray using getlantern/systray.
type desktopManager struct {
	cfg    Config
	status string
	paused bool

	mStatus   *systray.MenuItem
	mPause    *systray.MenuItem
	mOpen     *systray.MenuItem
	mSettings *systray.MenuItem
	mAuto     *systray.MenuItem
	mQuit     *systray.MenuItem
}

// New creates a desktop tray manager (requires CGO).
func New(cfg Config) Manager {
	return &desktopManager{cfg: cfg}
}

// Run starts the tray. Blocks until systray.Quit() is called.
func (m *desktopManager) Run() error {
	systray.Run(m.onReady, m.onExit)
	return nil
}

func (m *desktopManager) onReady() {
	slog.Info("[Tray] Initializing system tray")

	// Set icon (placeholder — 16x16 simple icon data)
	// In production, embed actual icon bytes here
	systray.SetTemplateIcon(sampleIcon, sampleIcon)
	systray.SetTooltip("livesync-sync")

	// Status (non-clickable display)
	m.mStatus = systray.AddMenuItem("Status: Starting...", "Current status")
	m.mStatus.Disable()

	systray.AddSeparator()

	// Pause/Resume toggle
	m.mPause = systray.AddMenuItem("Pause Sync", "Pause synchronization")

	// Open vault
	m.mOpen = systray.AddMenuItem("Open Vault", "Open vault folder")

	systray.AddSeparator()

	// Settings
	m.mSettings = systray.AddMenuItem("Settings...", "Open settings in browser")

	systray.AddSeparator()

	// Auto-start
	m.mAuto = systray.AddMenuItemCheckbox("Auto Start", "Run at login", m.cfg.Autostart)

	systray.AddSeparator()

	// Quit
	m.mQuit = systray.AddMenuItem("Quit", "Exit livesync-sync")

	// Start event handler goroutine
	go m.handleEvents()
}

func (m *desktopManager) handleEvents() {
	for {
		select {
		case <-m.mPause.ClickedCh:
			m.paused = !m.paused
			if m.paused {
				m.mPause.SetTitle("Resume Sync")
				m.SetStatus("Paused")
			} else {
				m.mPause.SetTitle("Pause Sync")
				m.SetStatus("Running")
			}

		case <-m.mOpen.ClickedCh:
			if m.cfg.OnOpenVault != nil {
				m.cfg.OnOpenVault()
			}

		case <-m.mSettings.ClickedCh:
			if m.cfg.OnSettings != nil {
				m.cfg.OnSettings()
			}

		case <-m.mAuto.ClickedCh:
			m.cfg.Autostart = !m.cfg.Autostart
			if m.cfg.Autostart {
				m.mAuto.Check()
			} else {
				m.mAuto.Uncheck()
			}

		case <-m.mQuit.ClickedCh:
			slog.Info("[Tray] Quit requested")
			systray.Quit()
			return
		}
	}
}

func (m *desktopManager) onExit() {
	slog.Info("[Tray] Exited")
}

func (m *desktopManager) Stop() {
	systray.Quit()
}

func (m *desktopManager) SetStatus(status string) {
	m.status = status
	if m.mStatus != nil {
		m.mStatus.SetTitle("Status: " + status)
	}
}

func (m *desktopManager) SetSyncStatus(running bool) {
	if running {
		m.SetStatus("● Running")
		if m.mPause != nil {
			m.mPause.SetTitle("Pause Sync")
		}
	} else {
		m.SetStatus("○ Stopped")
		if m.mPause != nil {
			m.mPause.SetTitle("Resume Sync")
		}
	}
}

func (m *desktopManager) ShowNotification(title, message string) error {
	slog.Info("[Tray] Notification", "title", title, "message", message)
	// systray doesn't have native notifications;
	// we log it for now. On macOS, we could use `growl` or similar.
	return nil
}

// sampleIcon is a minimal 16x16 PNG icon placeholder.
// Replace with a real icon in production.
var sampleIcon = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x10, 0x00, 0x00, 0x00, 0x10,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0xF3, 0xFF,
	0x61, 0x00, 0x00, 0x00, 0x01, 0x73, 0x52, 0x47,
	0x42, 0x00, 0xAE, 0xCE, 0x1C, 0xE9, 0x00, 0x00,
	0x00, 0x04, 0x67, 0x41, 0x4D, 0x41, 0x00, 0x00,
	0xB1, 0x8F, 0x0B, 0xFC, 0x61, 0x05, 0x00, 0x00,
	0x00, 0x09, 0x70, 0x48, 0x59, 0x73, 0x00, 0x00,
	0x0E, 0xC3, 0x00, 0x00, 0x0E, 0xC3, 0x01, 0xC7,
	0x6F, 0xA8, 0x64, 0x00, 0x00, 0x00, 0x1F, 0x49,
	0x44, 0x41, 0x54, 0x38, 0x4F, 0x63, 0x60, 0xA0,
	0x33, 0x60, 0xC0, 0x00, 0x8C, 0x8C, 0x8C, 0xE8,
	0x6A, 0x18, 0x18, 0x18, 0x18, 0x19, 0x18, 0x18,
	0x50, 0xC5, 0xF0, 0x62, 0x00, 0x00, 0x0F, 0x2C,
	0x10, 0x0F, 0x1A, 0x7F, 0xB5, 0x9B, 0x00, 0x00,
	0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE, 0x42,
	0x60, 0x82,
}
