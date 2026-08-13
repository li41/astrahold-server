# S3-D.2 Action Intent Integration

S3-D.2 將 Gate-specific attack path 收斂到 generic Combat Action contract。

- Client 只送 action_id、target_kind、target_id。
- Server Combat Action Catalog 是 damage、range、cooldown 的唯一真相來源。
- Gate domain 只負責 target-specific alive、Layer、Range、LOS、HP 與 blocker transaction。
- Reliable action sequence 表示 intent 已處理；gameplay rejection 不可重播。
