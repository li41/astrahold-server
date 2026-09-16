package gameplayworld

import (
	"fmt"

	"github.com/li41/astrahold-server/internal/world"
)

// MapID 是跨系統穩定的地圖 identity；不承載 Client scene、asset path 或 chunk identity。
type MapID string

type MapKind string

const (
	// MapIDGMRoom 是封閉 GM 密室的正式穩定 identity。
	MapIDGMRoom MapID = "map0"
	// MapIDStarterVillage 是玩家新手村的正式穩定 identity，也是舊存檔缺少 map identity 時的回退點。
	MapIDStarterVillage MapID = "map1"

	MapKindOverworld   MapKind = "overworld"
	MapKindUnderground MapKind = "underground"
	MapKindIsolated    MapKind = "isolated"
)

type MapScaleClass string

const (
	MapScaleSmall  MapScaleClass = "small"
	MapScaleMedium MapScaleClass = "medium"
	MapScaleLarge  MapScaleClass = "large"
)

// MapEnvelope 是已定稿的最大 X/Z 製作尺度，不代表整個矩形皆可走，也不自動建立 navigation surface。
type MapEnvelope struct {
	WidthX  float32 `json:"width_x"`
	LengthZ float32 `json:"length_z"`
}

// MapPlanningRange 保存 Client World Master 已鎖定、但尚未有精確 envelope 的規模分級。
// 這不是 movement bounds，不能當作精確矩形使用。
type MapPlanningRange struct {
	MinMeters float32 `json:"min_meters"`
	MaxMeters float32 `json:"max_meters"`
}

type MapEntrance struct {
	ID    string        `json:"id"`
	Layer world.LayerID `json:"layer"`
	Point PointXZ       `json:"point"`
}

// MapAuthority 保存 Server-owned map identity、入口與已正式定稿的核心尺度。
// 尚未 production 的地下空間只保存 planning range；不得因此生成可走 surface、portal 或 collision。
type MapAuthority struct {
	ID              MapID             `json:"id"`
	Kind            MapKind           `json:"kind"`
	ScaleClass      MapScaleClass      `json:"scale_class,omitempty"`
	Envelope        *MapEnvelope       `json:"envelope,omitempty"`
	PlanningRange   *MapPlanningRange  `json:"planning_range,omitempty"`
	Entrance        *MapEntrance       `json:"entrance,omitempty"`
	LevelCount      uint16             `json:"level_count,omitempty"`
	MaxDepthMeters  float32            `json:"max_depth_meters,omitempty"`
	MainRouteMeters float32            `json:"main_route_meters,omitempty"`
}

func (d Definition) Map(id MapID) (MapAuthority, bool) {
	for _, m := range d.Maps {
		if m.ID == id {
			return m, true
		}
	}
	return MapAuthority{}, false
}

func validateMaps(d Definition) error {
	ids := make(map[MapID]struct{}, len(d.Maps))
	entranceIDs := make(map[string]struct{}, len(d.Maps))
	for i, m := range d.Maps {
		if m.ID == "" {
			return fmt.Errorf("map[%d] id", i)
		}
		if _, exists := ids[m.ID]; exists {
			return fmt.Errorf("duplicate map id %q", m.ID)
		}
		ids[m.ID] = struct{}{}

		if m.ID == MapIDGMRoom && m.Kind != MapKindIsolated {
			return fmt.Errorf("map %q must be isolated", m.ID)
		}

		switch m.Kind {
		case MapKindOverworld:
			if m.Envelope == nil || m.PlanningRange != nil {
				return fmt.Errorf("map %q overworld requires exact envelope", m.ID)
			}
		case MapKindUnderground:
			if m.Entrance == nil {
				return fmt.Errorf("map %q underground entrance missing", m.ID)
			}
			if m.ScaleClass != MapScaleSmall && m.ScaleClass != MapScaleMedium && m.ScaleClass != MapScaleLarge {
				return fmt.Errorf("map %q invalid scale class %q", m.ID, m.ScaleClass)
			}
			if (m.Envelope == nil) == (m.PlanningRange == nil) {
				return fmt.Errorf("map %q requires exactly one of envelope/planning_range", m.ID)
			}
		case MapKindIsolated:
			if m.Envelope == nil || m.PlanningRange != nil {
				return fmt.Errorf("map %q isolated map requires exact envelope", m.ID)
			}
			if m.Entrance != nil {
				return fmt.Errorf("map %q isolated map cannot expose a world entrance", m.ID)
			}
		default:
			return fmt.Errorf("map %q invalid kind %q", m.ID, m.Kind)
		}

		if m.Envelope != nil && (!positiveFinite(m.Envelope.WidthX) || !positiveFinite(m.Envelope.LengthZ)) {
			return fmt.Errorf("map %q invalid envelope", m.ID)
		}
		if m.PlanningRange != nil {
			if !positiveFinite(m.PlanningRange.MinMeters) || !positiveFinite(m.PlanningRange.MaxMeters) || m.PlanningRange.MinMeters > m.PlanningRange.MaxMeters {
				return fmt.Errorf("map %q invalid planning range", m.ID)
			}
		}
		if m.MaxDepthMeters != 0 && !positiveFinite(m.MaxDepthMeters) {
			return fmt.Errorf("map %q invalid max depth", m.ID)
		}
		if m.MainRouteMeters != 0 && !positiveFinite(m.MainRouteMeters) {
			return fmt.Errorf("map %q invalid main route", m.ID)
		}

		if m.Entrance != nil {
			if m.Entrance.ID == "" || !finite(m.Entrance.Point.X) || !finite(m.Entrance.Point.Z) {
				return fmt.Errorf("map %q invalid entrance", m.ID)
			}
			if _, exists := entranceIDs[m.Entrance.ID]; exists {
				return fmt.Errorf("duplicate map entrance id %q", m.Entrance.ID)
			}
			entranceIDs[m.Entrance.ID] = struct{}{}
			if !pointCoveredBySurface(d.Surfaces, m.Entrance.Layer, m.Entrance.Point.X, m.Entrance.Point.Z) {
				return fmt.Errorf("map %q entrance is outside layer %d surfaces", m.ID, m.Entrance.Layer)
			}
		}
	}
	return nil
}
