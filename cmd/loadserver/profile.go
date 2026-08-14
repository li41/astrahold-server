package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
)

type allocProfiler struct {
	prefix       string
	previousRate int
}

func newAllocProfiler(prefix string, sampleRate int) (*allocProfiler, error) {
	if prefix == "" {
		return nil, nil
	}
	if sampleRate <= 0 {
		sampleRate = 64 * 1024
	}
	dir := filepath.Dir(prefix)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	profiler := &allocProfiler{prefix: prefix, previousRate: runtime.MemProfileRate}
	runtime.MemProfileRate = sampleRate
	return profiler, nil
}

func (p *allocProfiler) Close() {
	if p != nil {
		runtime.MemProfileRate = p.previousRate
	}
}

func (p *allocProfiler) Write(label string) error {
	if p == nil {
		return nil
	}
	if label == "" {
		return fmt.Errorf("loadserver: allocation profile label is required")
	}
	profile := pprof.Lookup("allocs")
	if profile == nil {
		return fmt.Errorf("loadserver: allocs profile unavailable")
	}
	path := fmt.Sprintf("%s-%s.pprof", p.prefix, label)
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := profile.WriteTo(file, 0); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
