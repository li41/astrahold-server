//go:build windows

package siegeownership

// syncDirectory is intentionally a no-op on Windows. Go's os.File.Sync on a directory
// handle returns ERROR_ACCESS_DENIED on supported Windows deployments, which previously made
// an otherwise successful temp-file fsync + atomic rename fail worldd startup. The record file
// itself is still fsynced before close and published with os.Rename. Unix-like production
// deployments retain the additional directory fsync in directory_sync_other.go.
func syncDirectory(string) error {
	return nil
}
