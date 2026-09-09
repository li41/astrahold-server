//go:build !windows

package siegeownership

import "os"

// syncDirectory persists the rename's directory entry on platforms where Go exposes
// directory fsync through os.File.Sync.
func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil {
		return syncErr
	}
	return closeErr
}
