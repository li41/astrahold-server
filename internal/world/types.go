// Package world 定義 Astrahold 世界模擬最底層的資料型別。
//
// 這個 package 刻意不依賴網路、資料庫、Godot 或特定導航實作，
// 讓其他 server subsystem 可以共同使用同一套世界座標語意。
package world

import "math"

// EntityID 是伺服器內實體的唯一識別碼。
type EntityID uint64

// LayerID 表示邏輯樓層／導航層。
//
// 即使兩個實體在 X/Z 投影上重疊，只要位於不同 Layer，仍可被世界模型區分。
type LayerID uint16

// Vec3 是伺服器使用的三維向量。
type Vec3 struct {
	X float32
	Y float32
	Z float32
}

// Position 是 Astrahold 的權威世界位置。
// X/Y/Z 採公尺語意；Layer 用於橋樑、城牆、地下層等邏輯拓樸。
type Position struct {
	X     float32
	Y     float32
	Z     float32
	Layer LayerID
}

// EntityKind 是伺服器端實體的大分類。
type EntityKind uint8

const (
	EntityUnknown EntityKind = iota
	EntityPlayer
	EntityNPC
	EntityMonster
	EntitySiegeObject
	EntityItemDrop
)

// Transform 是同步給其他系統的最小空間狀態。
type Transform struct {
	Position Position
	Yaw      float32
}

// EntityState 是世界層需要知道的最小實體狀態。
// ArchetypeID 只引用 immutable authored content identity；它不包含 model path、AI runtime 或 HP truth。
type EntityState struct {
	ID          EntityID
	Kind        EntityKind
	ArchetypeID string
	Transform   Transform
}

// Add 將位移向量加到 Position；Layer 不變。
func (p Position) Add(v Vec3) Position {
	p.X += v.X
	p.Y += v.Y
	p.Z += v.Z
	return p
}

// DistanceXZSquared 回傳忽略高度的水平距離平方。
// AOI 與多數 MMO 的粗略鄰近查詢通常以水平距離為主。
func (p Position) DistanceXZSquared(other Position) float32 {
	dx := p.X - other.X
	dz := p.Z - other.Z
	return dx*dx + dz*dz
}

// DistanceSquared 回傳完整 XYZ 距離平方。
func (p Position) DistanceSquared(other Position) float32 {
	dx := p.X - other.X
	dy := p.Y - other.Y
	dz := p.Z - other.Z
	return dx*dx + dy*dy + dz*dz
}

// NormalizedXZ 將 X/Z 正規化並忽略 Y。
// 零向量會回傳零向量。
func (v Vec3) NormalizedXZ() Vec3 {
	lengthSq := float64(v.X*v.X + v.Z*v.Z)
	if lengthSq == 0 {
		return Vec3{}
	}
	inv := float32(1 / math.Sqrt(lengthSq))
	return Vec3{X: v.X * inv, Z: v.Z * inv}
}

// Scale 將向量乘上 scalar。
func (v Vec3) Scale(scalar float32) Vec3 {
	return Vec3{X: v.X * scalar, Y: v.Y * scalar, Z: v.Z * scalar}
}
