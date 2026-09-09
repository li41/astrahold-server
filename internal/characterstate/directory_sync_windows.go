//go:build windows

package characterstate

// syncDirectory is intentionally a no-op on Windows. Go's os.File.Sync on a
// directory handle returns ERROR_ACCESS_DENIED on supported Windows deployments.
// Character record/checkpoint temp files are still fsynced before close and
// atomically published with os.Rename. Unix-like deployments retain the
// additional directory fsync in directory_sync_other.go.
func syncDirectory(string) error {
	return nil
}
