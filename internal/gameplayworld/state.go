package gameplayworld

// BlockerState 是 Gameplay World 的 runtime dynamic state。
// Definition 描述 bake 初始值；BlockerState 描述目前 World Runtime 真正啟用狀態。
type BlockerState struct {
	ID      string
	Enabled bool
}
