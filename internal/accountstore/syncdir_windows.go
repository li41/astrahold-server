//go:build windows

package accountstore

func syncDirectory(string) error {
	// The account file itself is synced before the atomic rename. Go's
	// os.File.Sync on directory handles is not supported reliably on Windows
	// and returns ERROR_ACCESS_DENIED, while the standard library exposes no
	// portable directory-fsync equivalent.
	return nil
}
