//go:build windows

package deathoutcome

// syncDirectory is intentionally a no-op on Windows. Go's os.File.Sync on a
// directory handle returns ERROR_ACCESS_DENIED on supported Windows deployments.
// The checkpoint temp file is still fsynced before close and then atomically
// published with os.Rename. Unix-like production deployments retain the
// additional directory fsync in directory_sync_other.go.
func syncDirectory(string) error {
	return nil
}
