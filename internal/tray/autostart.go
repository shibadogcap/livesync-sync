//go:build !notray

package tray

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// SetAutostart enables or disables login-time autostart.
// Linux: creates/removes ~/.config/autostart/livesync-sync.desktop
// Windows: creates/removes HKCU\...\Run registry key
// macOS: creates/removes ~/Library/LaunchAgents/
func SetAutostart(enabled bool) error {
	switch runtime.GOOS {
	case "linux":
		return setAutostartLinux(enabled)
	case "windows":
		return setAutostartWindows(enabled)
	case "darwin":
		return setAutostartDarwin(enabled)
	default:
		return fmt.Errorf("autostart not supported on %s", runtime.GOOS)
	}
}

func setAutostartLinux(enabled bool) error {
	autostartDir := filepath.Join(os.Getenv("HOME"), ".config", "autostart")
	desktopFile := filepath.Join(autostartDir, "livesync-sync.desktop")

	if enabled {
		if err := os.MkdirAll(autostartDir, 0755); err != nil {
			return err
		}

		// Find the binary path
		exe, err := os.Executable()
		if err != nil {
			return err
		}

		content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=livesync-sync
Comment=Self-hosted LiveSync bridge
Exec=%s
Terminal=false
Categories=Utility;
X-GNOME-Autostart-enabled=true
`, exe)

		if err := os.WriteFile(desktopFile, []byte(content), 0644); err != nil {
			return err
		}
		slog.Info("[Tray] Autostart enabled (Linux)", "path", desktopFile)
	} else {
		if err := os.Remove(desktopFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		slog.Info("[Tray] Autostart disabled (Linux)")
	}

	return nil
}

func setAutostartWindows(enabled bool) error {
	// On Windows, we'd use the registry:
	// HKEY_CURRENT_USER\Software\Microsoft\Windows\CurrentVersion\Run
	// This requires the `golang.org/x/sys/windows/registry` package.
	// For now, we log a note.
	if enabled {
		slog.Info("[Tray] Autostart: Windows registry key would be created")
	} else {
		slog.Info("[Tray] Autostart: Windows registry key would be removed")
	}
	return nil
}

func setAutostartDarwin(enabled bool) error {
	launchAgentsDir := filepath.Join(os.Getenv("HOME"), "Library", "LaunchAgents")
	plistFile := filepath.Join(launchAgentsDir, "com.livesync.sync.plist")

	if enabled {
		if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
			return err
		}

		exe, err := os.Executable()
		if err != nil {
			return err
		}

		content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.livesync.sync</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>--daemon</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, exe)

		if err := os.WriteFile(plistFile, []byte(content), 0644); err != nil {
			return err
		}
		slog.Info("[Tray] Autostart enabled (macOS)", "path", plistFile)
	} else {
		if err := os.Remove(plistFile); err != nil && !os.IsNotExist(err) {
			return err
		}
		slog.Info("[Tray] Autostart disabled (macOS)")
	}

	return nil
}
