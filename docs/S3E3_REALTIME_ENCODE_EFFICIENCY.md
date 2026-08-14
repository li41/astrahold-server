# S3-E.3：Realtime Encode / Wire Efficiency

S3-E.3 延續 S3-E.2 的量測結論：500-client Gate Zerg 已回到 20Hz 的 50ms tick budget 內，因此本階段不先假設必須修改 wire，也不預先把 Quantized / Delta 當成答案。

本階段先量測 realtime outbound 的實際頻寬、datagram 數與 encode CPU，再針對確定存在的 allocation / copy 成本做優化。

最終結果是：**Protocol v6、GameV1 wire layout、1200-byte UDP MTU 全部不變；Server 改成 per-writer reusable buffer + single-pass append encoding。**

## S3-E.2 起點

S3-E.2 最終 500-client Gate Zerg 已達：

```text
Tick p99                     32.71 ms
AOI avg                       0.824 ms
Replication Build avg         7.580 ms
TotalAlloc                  377.85 MB
```

這表示下一步不應再拆 mutable World ownership，也沒有 capacity 壓力要求立刻做 wire-incompatible payload redesign。

## 第一輪：先量 Wire / Encode Baseline

S3-E.3 先在不改 encoder 行為的情況下，為 Siege Load Lab 增加 transport metrics：

```text
Realtime datagrams
Realtime bytes
Snapshot datagrams / bytes
Correction datagrams / bytes
Encode total CPU
Encode average / datagram
```

量測 counters 在所有預期 Client ready 後才 reset，與既有 Server memory / tick measurement window 對齊。

### 500-client Gate Zerg baseline

舊 encoder：

```text
measurement                   10s
Realtime datagrams          127,009
Realtime throughput          53.16 Mbit/s
Snapshot throughput          49.56 Mbit/s
Correction throughput         3.60 Mbit/s
Encode total                343.01 ms / 10s
Encode avg                    2.70 us / datagram
TotalAlloc                  368.40 MB
Mallocs                       1.53M
```

平均到 500 個 Client，Server aggregate realtime traffic 約等於每 Client 106 kbit/s。

這份數據有兩個重要結論：

1. **Encode CPU 不是目前容量瓶頸。** 343ms / 10s 約等於單核心 3.4% CPU，沒有理由為此拆 World Actor 或改變 replication semantics。
2. **目前也沒有數據要求立刻 Quantized / Delta。** 53 Mbit/s 是這個 500-client hotspot regression 的 Server aggregate outbound，不代表 Internet production bandwidth 無成本，但在本階段更明確的問題是每 datagram 多重 allocation / copy。

## 舊 Realtime Encode Path

S3-E.2 的 realtime writer 使用：

```text
protocol.Message
        ↓
GameV1.Marshal
        ↓
new payload []byte
        ↓
transport.EncodeFrame
        ↓
new ASTR frame []byte
        ↓
tcpudp.EncodeDatagram
        ↓
new ASTU datagram []byte
        ↓
UDP WriteToUDP
```

因此一個 WorldSnapshot / PositionCorrection 在送出前，會經過 payload、frame、datagram 三段配置與 copy。

這些 allocation 不在 World Owner gameplay simulation 裡，但會增加 Server 總 allocation、GC pressure 與 writer-side memory bandwidth。

## 最終架構

S3-E.3 改為：

```text
client realtime writer goroutine
        ↓
owned 1200-byte scratch buffer
        ↓
AppendEncodeDatagram
├── reserve ASTU header
└── AppendEncodeEnvelope
    ├── reserve ASTR header
    └── GameV1.AppendMarshal
        └── payload directly into final datagram buffer
        ↓
fill ASTR header
        ↓
fill ASTU header
        ↓
UDP WriteToUDP
        ↓
reuse same writer-owned buffer
```

### AppendPayloadCodec

`transport.PayloadCodec` 的既有 contract 保留不變。

S3-E.3 新增 optional extension：

```go
type AppendPayloadCodec interface {
    AppendMarshal([]byte, protocol.Message) ([]byte, error)
}
```

如果 codec 支援 `AppendPayloadCodec`，transport 可以直接把 payload 寫入 caller 提供的 buffer；若不支援則 fallback 到既有 `Marshal`。

因此這不是把 transport 綁死在 GameV1，也不破壞 Protocol semantics / Transport separation。

### GameV1 Append Encode

GameV1 對 realtime message 實作 append path：

- `ClientMoveInput`
- `WorldSnapshot`
- `PositionCorrection`

原本的 `Marshal` / `Unmarshal` API 仍保留；Reliable JSON fallback 也不受影響。

### Buffer Ownership

每個 `clientConnection.runRealtimeWriter` 擁有自己的 1200-byte scratch buffer：

```text
1 Session
→ 1 realtime writer goroutine
→ 1 reusable datagram buffer
```

沒有跨 Session 共用 mutable buffer，也沒有 blanket `sync.Pool`。

`net.UDPConn.WriteToUDP` 在呼叫回傳前同步消費 caller slice，因此 writer 只在 `WriteToUDP` 回傳後重用該 buffer。

這個 ownership boundary 很重要：如果未來 transport 換成會在 API return 後仍持有 caller memory 的 async backend，不能直接沿用此 reuse 假設，必須重新定義 buffer lifetime。

## Wire Compatibility

S3-E.3 沒有改：

- Protocol Version：v6
- ASTU datagram header
- ASTR frame header
- GameV1 WorldSnapshot header
- GameV1 EntityTransform：26 bytes
- PositionCorrection layout
- WorldSnapshot chunk semantics
- MaxDatagramSize：1200 bytes
- `MaxSnapshotEntitiesPerChunk`

單元測試會用舊的 `transport.EncodeEnvelope` 組出 legacy datagram，再與 `AppendEncodeDatagram` 做 byte-for-byte equality 驗證。

另外使用預先配置 1200-byte buffer 對 PositionCorrection 跑 `testing.AllocsPerRun`，要求 reusable append encode path 為 0 allocation。

## 24-client Vertical Siege A/B

| 指標 | 舊 encoder | Single-pass | 變化 |
|---|---:|---:|---:|
| Tick avg | 0.117 ms | 0.084 ms | -28.5% |
| Tick p99 | 0.393 ms | 0.202 ms | -48.7% |
| TotalAlloc | 5.75 MB | 4.62 MB | -19.7% |
| Mallocs | 34,788 | 27,594 | -20.7% |
| Realtime | 0.662 Mbit/s | 0.662 Mbit/s | ~0% |
| Encode avg | 4.25 us | 0.32 us | -92.4% |

Wire volume 完全等量：

```text
Realtime datagrams     2,400 → 2,400
Realtime bytes       413,660 → 413,660
```

## 100-client Gate Zerg A/B

| 指標 | 舊 encoder | Single-pass | 變化 |
|---|---:|---:|---:|
| Tick avg | 0.584 ms | 0.615 ms | +5.3% |
| Tick p99 | 4.317 ms | 4.349 ms | +0.7% |
| TotalAlloc | 27.94 MB | 16.70 MB | -40.2% |
| Mallocs | 221,380 | 171,645 | -22.5% |
| Realtime | 3.922 Mbit/s | 3.919 Mbit/s | -0.1% |
| Encode avg | 1.36 us | 0.43 us | -68.5% |

Tick 差異落在 Hosted Runner noise 範圍，allocation 與 encode CPU 則有明確方向性改善。

## 500-client Gate Zerg A/B

| 指標 | 舊 encoder | Single-pass | 變化 |
|---|---:|---:|---:|
| completed ticks / 10s | 200 | 202 | +1.0% |
| Tick avg | 8.234 ms | 7.732 ms | -6.1% |
| Tick p99 | 19.403 ms | 19.825 ms | +2.2% |
| AOI avg | 0.741 ms | 0.584 ms | -21.2% |
| Replication Build avg | 6.600 ms | 6.194 ms | -6.1% |
| TotalAlloc | 368.40 MB | 178.39 MB | **-51.6%** |
| Mallocs | 1.527M | 1.251M | **-18.1%** |
| Realtime throughput | 53.164 Mbit/s | 53.241 Mbit/s | +0.1% |
| Encode total / 10s | 343.01 ms | 61.51 ms | **-82.1%** |
| Encode avg | 2.70 us | 0.48 us | **-82.3%** |

Realtime volume 的小幅差異來自該 run 完成 202 ticks、snapshot scheduling 數量略高，不是 wire layout 變小：

```text
baseline realtime datagrams       127,009
single-pass realtime datagrams    128,539

baseline realtime bytes        66,457,594
single-pass realtime bytes      66,578,542
```

即使 single-pass run 實際送出更多 datagrams，TotalAlloc 仍下降約 190 MB。

## 500-client Correctness Gate

Single-pass 最終 run：

```text
connected / ready              500 / 500
Spawn                    250,000 / 250,000
Reliable messages            500,500
Decode errors                      0
Network errors                     0
Delivery errors                    0
Datagram too large                 0
Incomplete snapshot resets         0
```

因此 reusable buffer 沒有破壞：

- Reliable lifecycle
- WorldSnapshot chunk completion
- partial update batch semantics
- PositionCorrection
- realtime mailbox coalescing
- 1200-byte MTU gate

## 為什麼本階段不做 Quantized / Delta

S3-E.3 的 baseline 已量到真實 500-client hotspot wire rate，但目前沒有證據顯示必須付出 wire-incompatible complexity：

- S3-E.2 已通過 20Hz 50ms p99 capacity gate
- encode CPU 很低
- single-pass 在不改 wire 下已把 500-client TotalAlloc 再降低約 52%
- Client 不需要 contract 改動

Quantized / Delta 會引入新的 correctness 與 operational surface，例如：

- quantization range / precision contract
- baseline identity / reset
- packet loss 後 delta recovery
- protocol version negotiation
- Client decode / interpolation compatibility

在 bandwidth、WAN egress cost、更多玩家、或更高 transform cadence 尚未成為實測限制前，不應只為了「看起來更壓縮」提前承擔這些複雜度。

## S3-E.3 結論

S3-E.3 的實測支持以下決策：

1. 保持 Protocol v6。
2. 保持 1200-byte MTU。
3. 保持 GameV1 EntityTransform layout。
4. Realtime writer 使用 per-connection reusable MTU buffer。
5. Transport 透過 optional append codec extension 支援 single-pass encoding，不把 protocol semantics 綁進 transport。
6. Client 不需要任何修改。
7. 不因目前 53 Mbit/s aggregate hotspot 數據自動進入 Quantized / Delta。

下一階段應再次以新的 workload / bandwidth / allocation profile 決定方向。若目標是繼續降低 Server allocation，應先 profile 剩餘的 `WorldSnapshot` transform materialization、realtime mailbox ownership 與 decode-side allocation；若目標轉為 Internet egress / 大型戰場頻寬，再評估 Quantized / Delta 或 shared serialization。
