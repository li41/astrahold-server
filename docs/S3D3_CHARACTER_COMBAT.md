# S3-D.3：Character Combat Target

S3-D.3 的目的，是讓 Protocol v5 建立的 generic `ClientUseAction` 第一次真正作用在非 Gate 目標。

本階段建立 Server-authoritative Character HP / Defeated target domain，並沿用 Combat Action Catalog 的 damage、range 與 cooldown。Character state 不放進 Navigation，也不把 HP 混進 transform snapshot。

Gameplay World `gate.attack` 的 legacy schema cleanup 不與本階段綁定；先驗證 entity target correctness，migration cleanup 另做。
