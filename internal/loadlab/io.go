package loadlab

import (
	"os"
	"path/filepath"
)

func WriteReport(path string, value any) error {
	data, err := MarshalReport(value)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
