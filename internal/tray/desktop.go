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

	systray.SetTemplateIcon(sampleIcon, sampleIcon)
	systray.SetTooltip("livesync-sync")

	// Status display — use cached status if set before onReady
	statusLabel := m.status
	if statusLabel == "" {
		statusLabel = "Starting..."
	}
	m.mStatus = systray.AddMenuItem("Status: "+statusLabel, "Current status")
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
				if m.cfg.OnPause != nil {
					m.cfg.OnPause()
				}
			} else {
				m.mPause.SetTitle("Pause Sync")
				m.SetStatus("Running")
				if m.cfg.OnResume != nil {
					m.cfg.OnResume()
				}
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
			if err := SetAutostart(m.cfg.Autostart); err != nil {
				slog.Warn("[Tray] Autostart toggle failed", "error", err)
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

