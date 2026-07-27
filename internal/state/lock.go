package state

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Lock provides cross-platform single-instance locking.
// On Linux: flock(2) via O_RDWR|O_CREAT|O_CLOEXEC + write PID
// On Windows: O_EXCL via O_CREAT|O_EXCL (atomic create)
type Lock struct {
	file *os.File
	path string
}

// NewLock acquires an exclusive lock file.
// Returns an error if another instance is already running.
func NewLock(path string) (*Lock, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("lock dir: %w", err)
	}

	// O_RDWR | O_CREAT | O_EXCL — atomic create, fails if file exists
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			// Check if the lock file is stale (process no longer exists)
			pidBytes, readErr := os.ReadFile(path)
			if readErr == nil && len(pidBytes) > 0 {
				var pid int
				if _, parseErr := fmt.Sscanf(string(pidBytes), "%d", &pid); parseErr == nil {
					process, procErr := os.FindProcess(pid)
					if procErr == nil {
						// Signal 0 checks if process exists without actually signaling
						if killErr := process.Signal(syscall.Signal(0)); killErr != nil {
							// Process is dead — remove stale lock
							os.Remove(path)
							f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
							if err == nil {
								goto acquired
							}
						}
					}
				}
			}
			return nil, fmt.Errorf("lock file exists: %s (is another instance running?)", path)
		}
		return nil, fmt.Errorf("lock acquire: %w", err)
	}

acquired:
	// Write our PID into the lock file for stale detection
	pid := os.Getpid()
	fmt.Fprintf(f, "%d\n", pid)
	f.Sync()

	return &Lock{file: f, path: path}, nil
}

// Unlock releases the lock and removes the lock file.
func (l *Lock) Unlock() error {
	if l.file == nil {
		return nil
	}
	l.file.Close()
	os.Remove(l.path)
	l.file = nil
	return nil
}
