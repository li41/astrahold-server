//go:build windows

package characterstate

func syncDirectory(string) error {
	// Character-state files and checkpoints are synced before atomic rename.
	// Go's os.File.Sync on directory handles is not supported reliably on
	// Windows and returns ERROR_ACCESS_DENIED, while the standard library
	// exposes no portable directory-fsync equivalent.
	return nil
}
