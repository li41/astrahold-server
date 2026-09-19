package characterstate

// defaultSnapshotMap applies the Server's only implicit map fallback: an in-memory
// character state that has not selected a map yet starts in map1 (starter village).
// Durable current-schema reads still validate the encoded map_id separately.
func defaultSnapshotMap(snapshot Snapshot) Snapshot {
	if snapshot.World.MapID == "" {
		snapshot.World.MapID = LegacyDefaultMapID
	}
	return snapshot
}

func defaultSaveIntentMap(intent SaveIntent) SaveIntent {
	intent.Snapshot = defaultSnapshotMap(intent.Snapshot)
	return intent
}
