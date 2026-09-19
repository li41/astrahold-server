package main

import (
	"fmt"
	"path/filepath"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/terrainheight"
)

func loadWorldHeightfields(worldPath string, definition gameplayworld.Definition) (map[string]*terrainheight.Field, error) {
	if len(definition.Heightfields) == 0 {
		return nil, nil
	}
	baseDir := filepath.Dir(worldPath)
	fields := make(map[string]*terrainheight.Field, len(definition.Heightfields))
	for _, binding := range definition.Heightfields {
		manifestPath := filepath.Join(baseDir, filepath.FromSlash(binding.Manifest))
		loaded, err := terrainheight.LoadFile(manifestPath, binding.ShellID)
		if err != nil {
			return nil, fmt.Errorf("surface %q manifest %q shell %q: %w", binding.SurfaceID, binding.Manifest, binding.ShellID, err)
		}
		if loaded.DataSHA256 != binding.DataSHA256 {
			return nil, fmt.Errorf("surface %q terrain hash got=%s want=%s", binding.SurfaceID, loaded.DataSHA256, binding.DataSHA256)
		}
		if _, exists := fields[binding.SurfaceID]; exists {
			return nil, fmt.Errorf("duplicate surface heightfield %q", binding.SurfaceID)
		}
		fields[binding.SurfaceID] = loaded.Field
	}
	return fields, nil
}
