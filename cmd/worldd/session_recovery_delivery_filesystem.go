package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/li41/astrahold-server/internal/accountrecovery"
)

const sessionRecoveryFilesystemAdapterMethod = "filesystem-reference-v1"

type filesystemRecoveryDeliveryAdapter struct {
	root     string
	revision string
}

func init() {
	if providerFlag := flag.CommandLine.Lookup("session-recovery-provider-file"); providerFlag != nil {
		providerFlag.Usage = "Optional public recovery provider config: schema v1 digest-only recovery code or schema v2 verified delivery reference adapter; requires durable account schema v4"
	}
}

func newFilesystemRecoveryDeliveryAdapter(root, revision string) (*filesystemRecoveryDeliveryAdapter, error) {
	root = strings.TrimSpace(root)
	revision = strings.TrimSpace(revision)
	if root == "" || revision == "" {
		return nil, fmt.Errorf("%w: filesystem recovery delivery root/revision must be non-empty", errSessionLoginConfig)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("%w: create filesystem recovery delivery root: %v", errSessionLoginConfig, err)
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("%w: stat filesystem recovery delivery root: %v", errSessionLoginConfig, err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%w: filesystem recovery delivery root must be a real directory", errSessionLoginConfig)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%w: filesystem recovery delivery root must not grant group/other permissions", errSessionLoginConfig)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve filesystem recovery delivery root: %v", errSessionLoginConfig, err)
	}
	return &filesystemRecoveryDeliveryAdapter{root: absolute, revision: revision}, nil
}

func (a *filesystemRecoveryDeliveryAdapter) Method() string {
	return sessionRecoveryFilesystemAdapterMethod
}

func (a *filesystemRecoveryDeliveryAdapter) Revision() string {
	if a == nil {
		return ""
	}
	return a.revision
}

func (a *filesystemRecoveryDeliveryAdapter) Deliver(ctx context.Context, delivery accountrecovery.Delivery) error {
	if a == nil || a.root == "" || !delivery.Valid() || !validFilesystemRecoveryDestination(delivery.Destination) || strings.ContainsAny(string(delivery.Proof), "\r\n") {
		return accountrecovery.ErrDeliveryPermanent
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	temporary, err := os.CreateTemp(a.root, ".recovery-delivery-*")
	if err != nil {
		return fmt.Errorf("%w: create temporary delivery: %v", accountrecovery.ErrDeliveryTransient, err)
	}
	temporaryPath := temporary.Name()
	keep := false
	defer func() {
		_ = temporary.Close()
		if !keep {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("%w: chmod temporary delivery: %v", accountrecovery.ErrDeliveryTransient, err)
	}

	payload := make([]byte, len(delivery.Proof)+1)
	copy(payload, delivery.Proof)
	payload[len(payload)-1] = '\n'
	defer clear(payload)
	if _, err := temporary.Write(payload); err != nil {
		return fmt.Errorf("%w: write temporary delivery: %v", accountrecovery.ErrDeliveryTransient, err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("%w: sync temporary delivery: %v", accountrecovery.ErrDeliveryTransient, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("%w: close temporary delivery: %v", accountrecovery.ErrDeliveryTransient, err)
	}

	finalPath := filepath.Join(a.root, delivery.Destination+".proof")
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return fmt.Errorf("%w: publish delivery: %v", accountrecovery.ErrDeliveryTransient, err)
	}
	keep = true
	if info, err := os.Stat(finalPath); err != nil || info.Mode().Perm() != 0o600 {
		if err == nil {
			err = errors.New("unexpected file permissions")
		}
		_ = os.Remove(finalPath)
		return fmt.Errorf("%w: verify published delivery: %v", accountrecovery.ErrDeliveryTransient, err)
	}
	return nil
}

func validFilesystemRecoveryDestination(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > accountrecovery.MaxDeliveryDestinationBytes || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}

var _ accountrecovery.DeliveryAdapter = (*filesystemRecoveryDeliveryAdapter)(nil)
