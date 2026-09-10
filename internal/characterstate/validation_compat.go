package characterstate

// validateSnapshot is kept as the package-local enqueue validation boundary used by
// Outbox. New intents must satisfy the current writer schema even though Store.Load
// remains backward-compatible with legacy records.
func validateSnapshot(snapshot Snapshot) error {
	return validateSnapshotV7(snapshot)
}
