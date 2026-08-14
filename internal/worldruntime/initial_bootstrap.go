package worldruntime

const (
	initialBootstrapUnseen uint8 = iota
	initialBootstrapActive
	initialBootstrapComplete
)

func (r *Runtime) suppressInitialBootstrapSnapshots() bool {
	return r.initialBootstrapState == initialBootstrapActive && !r.lifecycleChurnActive
}

// observeInitialBootstrapSnapshot 將 suppression 嚴格限制在第一次 startup lifecycle baseline。
// 第一次真的看到 Spawn/Despawn work 後進入 active；等 lifecycle snapshot 已無工作，且
// Initial Vitals pending也清空，就永久進入 complete。之後 late join不重新啟動；若 startup
// 尚未完成前已出現 Despawn churn，也直接結束 initial-only 模式，避免改寫正式 churn workload。
func (r *Runtime) observeInitialBootstrapSnapshot(sessionCount int, hadLifecycleWork bool) {
	if r.initialBootstrapState == initialBootstrapComplete {
		return
	}
	if r.lifecycleChurnActive {
		r.initialBootstrapState = initialBootstrapComplete
		return
	}
	if sessionCount <= 0 {
		return
	}
	if hadLifecycleWork {
		if r.initialBootstrapState == initialBootstrapUnseen {
			r.initialBootstrapState = initialBootstrapActive
		}
		return
	}
	if r.initialBootstrapState == initialBootstrapActive && len(r.sessionVitalsPending) == 0 {
		r.initialBootstrapState = initialBootstrapComplete
	}
}
