package characterstate

import "reflect"

// SnapshotsEqual compares durable gameplay snapshots semantically. Warehouse items are a slice,
// so Snapshot intentionally is not a Go-comparable type after warehouse persistence was added.
// Normalize the warehouse representation first so nil/empty slice encoding differences do not
// turn an otherwise identical durable snapshot into a replay conflict.
func SnapshotsEqual(a, b Snapshot) bool {
	a.Warehouse = warehouseStateForEquality(a.Warehouse)
	b.Warehouse = warehouseStateForEquality(b.Warehouse)
	return reflect.DeepEqual(a, b)
}

func SaveIntentsEqual(a, b SaveIntent) bool {
	return a.IntentID == b.IntentID && a.Identity == b.Identity && SnapshotsEqual(a.Snapshot, b.Snapshot)
}

func RecordsEqual(a, b Record) bool {
	return a.SchemaVersion == b.SchemaVersion && a.CharacterID == b.CharacterID && a.Revision == b.Revision && SnapshotsEqual(a.Snapshot, b.Snapshot)
}

func warehouseStateForEquality(state WarehouseState) WarehouseState {
	canonical, err := CanonicalWarehouseState(state)
	if err == nil {
		return canonical
	}
	return state
}
