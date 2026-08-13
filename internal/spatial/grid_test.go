package spatial

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestGridQueryRadiusAndLayer(t *testing.T) {
	grid := NewGrid(10)
	grid.Upsert(1, world.Position{X: 0, Y: 0, Z: 0, Layer: 0})
	grid.Upsert(2, world.Position{X: 5, Y: 1, Z: 0, Layer: 0})
	grid.Upsert(3, world.Position{X: 5, Y: 8, Z: 0, Layer: 1})
	grid.Upsert(4, world.Position{X: 30, Y: 0, Z: 0, Layer: 0})

	got := grid.QueryRadius(world.Position{Layer: 0}, 10, QueryOptions{SameLayer: true})
	want := []world.EntityID{1, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("QueryRadius() = %v, want %v", got, want)
	}

	got = grid.QueryRadius(world.Position{Layer: 0}, 10, QueryOptions{MaxHeightDelta: 2})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("height-filtered QueryRadius() = %v, want %v", got, want)
	}
}

func TestGridUpsertMovesBetweenCells(t *testing.T) {
	grid := NewGrid(10)
	grid.Upsert(7, world.Position{X: 1, Z: 1})
	grid.Upsert(7, world.Position{X: 25, Z: 1})

	nearOrigin := grid.QueryRadius(world.Position{}, 5, QueryOptions{})
	if len(nearOrigin) != 0 {
		t.Fatalf("old cell still contains entity: %v", nearOrigin)
	}

	nearNewPosition := grid.QueryRadius(world.Position{X: 25}, 1, QueryOptions{})
	if len(nearNewPosition) != 1 || nearNewPosition[0] != 7 {
		t.Fatalf("new cell query = %v, want [7]", nearNewPosition)
	}
}
