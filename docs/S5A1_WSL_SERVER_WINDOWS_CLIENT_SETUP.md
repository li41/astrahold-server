# S5-A.1 - WSL Server / Windows Client 安裝與編譯手冊

這份文件提供 Astrahold S5-A.1 Playable Siege MVP 的 **從零開始安裝、編譯、啟動與第一次攻城測試流程**。

目標拓樸：

```text
Windows 11 / Windows 10
├─ WSL2 / Ubuntu
│  └─ astrahold-server
│     ├─ worldd
│     ├─ accountctl
│     ├─ TLS 1.3 login endpoint
│     ├─ TLS 1.3 game bootstrap endpoint
│     └─ realtime UDP
│
└─ Windows native
   └─ astrahold-client
      ├─ .NET 8
      ├─ Godot 4.7.1 .NET
      └─ normal Main.tscn
```

這不是 public registration 流程。測試帳號由 operator 使用 `accountctl` 預先建立：

| Login / CharacterID | Siege Team |
| --- | --- |
| `playtest-attacker` | attacker |
| `playtest-defender` | defender |

Client 不選擇 team；Server 依 `config/siege-match-playtest.json` 決定角色所屬攻守方。

---

## 1. 版本需求

目前 repository contract：

### Server

- Go **1.26.x**
- Git
- OpenSSL
- Ubuntu / WSL2

`go.mod` 的 Go language/toolchain baseline 是 `go 1.26`。

### Client

- Windows x64
- .NET SDK **8.x**
- Godot Engine **4.7.1 .NET**
- Git

Client project：

```xml
<Project Sdk="Godot.NET.Sdk/4.7.1">
  <PropertyGroup>
    <TargetFramework>net8.0</TargetFramework>
  </PropertyGroup>
</Project>
```

> 請下載 **Godot .NET / C# 版本**，不要下載標準版 Godot，否則 C# project 無法正常 build/run。

官方參考：

- WSL: https://learn.microsoft.com/windows/wsl/install
- Go: https://go.dev/doc/install
- .NET on Windows: https://learn.microsoft.com/dotnet/core/install/windows
- Godot downloads: https://godotengine.org/download/

---

# Part A - 安裝 WSL Server 環境

## 2. Windows 安裝 WSL2 / Ubuntu

如果已經有 Ubuntu WSL2，可以跳過本節。

以 **系統管理員 PowerShell** 執行：

```powershell
wsl --install
```

若要明確指定 Ubuntu：

```powershell
wsl --install -d Ubuntu
```

安裝完成後重新啟動 Windows。

確認：

```powershell
wsl --status
wsl --list --verbose
```

正常應看到 Ubuntu 使用 version 2，例如：

```text
NAME      STATE    VERSION
Ubuntu    Running  2
```

進入 Ubuntu：

```powershell
wsl -d Ubuntu
```

第一次進入 Ubuntu 時建立 Linux username/password。

---

## 3. Ubuntu 安裝基本工具

在 WSL Ubuntu：

```bash
sudo apt update
sudo apt install -y \
  git \
  curl \
  ca-certificates \
  openssl \
  tar
```

確認：

```bash
git --version
openssl version
curl --version
```

---

## 4. 安裝 Go 1.26.x

Ubuntu repository 的 `golang-go` 不一定會是目前 Server 要求的 Go 1.26，因此建議使用 Go 官方 binary distribution。

先確認 CPU architecture：

```bash
uname -m
```

一般 Windows x64 / WSL2 應為：

```text
x86_64
```

以下以 Go 1.26.0 為安裝範例。若 go.dev 已提供更新的 1.26.x patch release，可以把 `GO_VERSION` 改成該版本。

```bash
cd /tmp

GO_VERSION=1.26.0
curl -LO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"

sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
```

加入 PATH：

```bash
grep -q '/usr/local/go/bin' ~/.bashrc || \
  echo 'export PATH=/usr/local/go/bin:$PATH' >> ~/.bashrc

source ~/.bashrc
```

確認：

```bash
go version
```

應看到 `go1.26.x linux/amd64`。

如果不是 1.26.x，先修正 Go 再繼續 build Server。

---

# Part B - 編譯 Astrahold Server

## 5. Clone Server repository

建議把 Linux build repository 放在 WSL filesystem，例如 `~/src`，不要放 `/mnt/c/...`；Linux toolchain 在 WSL filesystem 通常有更好的 I/O 表現。

```bash
mkdir -p ~/src
cd ~/src

git clone https://github.com/li41/astrahold-server.git
cd astrahold-server

git checkout main
git pull --ff-only
```

確認版本：

```bash
git rev-parse --short HEAD
go version
```

---

## 6. 下載 Go modules 並執行基本驗證

```bash
go mod download
go test ./...
```

如果只是第一次快速 playtest，可以先至少確認：

```bash
go test ./internal/siege/...
```

---

## 7. 編譯 production Server tools

在 `astrahold-server` root：

```bash
mkdir -p .tools/playtest

go build -o .tools/playtest/worldd ./cmd/worldd
go build -o .tools/playtest/accountctl ./cmd/accountctl
```

確認 binary：

```bash
ls -lh .tools/playtest/
.tools/playtest/worldd -h
.tools/playtest/accountctl -h
```

預期至少有：

```text
.tools/playtest/worldd
.tools/playtest/accountctl
```

---

# Part C - 建立 Playtest 帳號與本機資料

## 8. 建立 S5-A.1 playtest data root

```bash
cd ~/src/astrahold-server

export ASTRAHOLD_PLAYTEST_ROOT="$PWD/.playtest/siege"

rm -rf "$ASTRAHOLD_PLAYTEST_ROOT"
mkdir -p \
  "$ASTRAHOLD_PLAYTEST_ROOT/tls" \
  "$ASTRAHOLD_PLAYTEST_ROOT/character-state"
```

建立 account store：

```bash
.tools/playtest/accountctl init \
  -path "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json"
```

---

## 9. 建立攻方帳號

不要把 password 放在 shell command argument。

```bash
read -r -s -p 'Attacker password: ' ATTACKER_PASSWORD
printf '\n'

printf '%s\n' "$ATTACKER_PASSWORD" | \
.tools/playtest/accountctl create \
  -path "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  -login playtest-attacker \
  -character playtest-attacker \
  -password-stdin

unset ATTACKER_PASSWORD
```

第一次 solo smoke test 只有這個帳號就足夠。

---

## 10. 建立守方帳號

要測 PvP / contest / death / respawn / Victory / Defeat 時再建立：

```bash
read -r -s -p 'Defender password: ' DEFENDER_PASSWORD
printf '\n'

printf '%s\n' "$DEFENDER_PASSWORD" | \
.tools/playtest/accountctl create \
  -path "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  -login playtest-defender \
  -character playtest-defender \
  -password-stdin

unset DEFENDER_PASSWORD
```

---

# Part D - Windows Client 連 WSL Server 的網路設定

## 11. 取得 WSL IP

因為 Astrahold game session 在 TLS/TCP bootstrap 後還會使用 realtime UDP，所以第一次跨 Windows/WSL 測試建議直接使用 **WSL VM IP**，不要把測試建立在 localhost UDP forwarding 行為上。

在 WSL：

```bash
hostname -I
```

或 Windows PowerShell：

```powershell
wsl hostname -I
```

例如：

```text
172.28.112.45
```

在 WSL 設定：

```bash
export WSL_IP=172.28.112.45
```

請換成實際 IP。

> WSL restart 後 IP 可能改變。如果 IP 改變，要重新設定 Client environment；如果 TLS certificate SAN 只包含舊 IP，則也要重新產生測試 certificate。

---

# Part E - 建立本機測試 TLS

## 12. 建立 local CA

這組 CA 只供本機 / 開發 playtest 使用，不可當正式 production PKI。

```bash
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.key" \
  -out "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.crt" \
  -days 7 -sha256 \
  -subj '/CN=Astrahold Local Siege Playtest CA' \
  -addext 'basicConstraints=critical,CA:TRUE' \
  -addext 'keyUsage=critical,keyCertSign,cRLSign'
```

---

## 13. 建立 Server certificate

```bash
openssl req -newkey rsa:2048 -nodes \
  -keyout "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key" \
  -out "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.csr" \
  -subj '/CN=astrahold-wsl'
```

產生 certificate extensions，將 WSL IP 放進 SAN：

```bash
cat >"$ASTRAHOLD_PLAYTEST_ROOT/tls/server.ext" <<EOF
subjectAltName=DNS:localhost,IP:127.0.0.1,IP:${WSL_IP}
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
EOF
```

簽發：

```bash
openssl x509 -req \
  -in "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.csr" \
  -CA "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.crt" \
  -CAkey "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.key" \
  -CAcreateserial \
  -out "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -days 7 -sha256 \
  -extfile "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.ext"
```

檢查 SAN：

```bash
openssl x509 \
  -in "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -noout -subject -issuer -ext subjectAltName
```

---

## 14. 保護 private files

```bash
chmod 600 \
  "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.key" \
  "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key"
```

Client 只需要：

```text
ca.crt
```

不要把以下檔案複製到 Client：

```text
ca.key
server.key
```

---

# Part F - 啟動 production worldd

## 15. 啟動 WSL Server

從 repository root 啟動，讓 world/combat/respawn 等相對路徑可以正常 resolve。

```bash
cd ~/src/astrahold-server

.tools/playtest/worldd \
  -tcp 127.0.0.1:27777 \
  -udp 0.0.0.0:27778 \
  -siege-match config/siege-match-playtest.json \
  -session-login-account-file "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  -session-login-tls-listen 0.0.0.0:27444 \
  -session-login-tls-cert "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -session-login-tls-key "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key" \
  -trusted-tls-listen 0.0.0.0:27443 \
  -trusted-tls-cert "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -trusted-tls-key "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key" \
  -character-state-dir "$ASTRAHOLD_PLAYTEST_ROOT/character-state" \
  -character-state-save-journal "$ASTRAHOLD_PLAYTEST_ROOT/character-state-saves.journal" \
  -character-state-save-checkpoint "$ASTRAHOLD_PLAYTEST_ROOT/character-state-saves.checkpoint.json" \
  -death-outcome-journal "$ASTRAHOLD_PLAYTEST_ROOT/death-outcomes.journal" \
  -death-outcome-checkpoint "$ASTRAHOLD_PLAYTEST_ROOT/death-outcomes.checkpoint.json" \
  -siege-ownership-dir "$ASTRAHOLD_PLAYTEST_ROOT/siege-ownership"
```

這裡刻意：

- raw backend TCP `27777` 只 bind `127.0.0.1`；
- realtime UDP `27778` bind `0.0.0.0`，讓 Windows Client 可連 WSL；
- TLS login `27444` bind `0.0.0.0`；
- TLS game ingress `27443` bind `0.0.0.0`。

不要把 raw development TCP backend 直接 expose 到 Internet。

---

## 16. Server 正常啟動的 log

應看到包含：

```text
Astrahold worldd ready: protocol=9
siege_match_revision=s5a1-playtest-001
siege_match=castle-sandbox-siege-playtest
breach_gate=main-gate throne=throne
session login issuance: enabled=true
trusted TLS ingress: enabled=true
```

另一個 WSL terminal 可檢查 listeners：

```bash
ss -lntp | grep -E '27443|27444|27777'
ss -lnup | grep 27778
```

應有：

```text
TCP 27443
TCP 27444
TCP 27777
UDP 27778
```

---

# Part G - 安裝 Windows Client 環境

## 17. 安裝 Git

如果已安裝可以跳過。

PowerShell：

```powershell
winget install --id Git.Git -e
```

確認：

```powershell
git --version
```

---

## 18. 安裝 .NET SDK 8

Client target 是 `net8.0`，請安裝 SDK，不只安裝 Runtime。

使用 WinGet：

```powershell
winget install --id Microsoft.DotNet.SDK.8 -e
```

重新開一個 PowerShell 後確認：

```powershell
dotnet --info
dotnet --list-sdks
```

應至少看到一個 8.x SDK。

---

## 19. 安裝 Godot 4.7.1 .NET

到 Godot 官方下載頁下載：

```text
Godot Engine - .NET 4.7.1
Windows x86_64
```

下載完成後解壓，例如：

```text
C:\Tools\Godot\4.7.1-dotnet\
```

確認 executable，例如：

```text
C:\Tools\Godot\4.7.1-dotnet\Godot_v4.7.1-stable_mono_win64.exe
```

可選：把 Godot 目錄加入 Windows PATH；或直接使用完整 executable path。

確認：

```powershell
godot --version
```

如果沒加 PATH：

```powershell
& 'C:\Tools\Godot\4.7.1-dotnet\Godot_v4.7.1-stable_mono_win64.exe' --version
```

> Astrahold Client 使用 C#，因此一定要使用 Godot **.NET / mono** build。

---

# Part H - 編譯 Astrahold Client

## 20. Clone Client repository

Windows PowerShell：

```powershell
mkdir C:\src -ErrorAction SilentlyContinue
cd C:\src

git clone https://github.com/li41/astrahold-client.git
cd astrahold-client

git checkout main
git pull --ff-only
```

---

## 21. Restore / build C# Client

```powershell
dotnet restore Astrahold.Client.csproj
dotnet build Astrahold.Client.csproj --configuration Debug
```

正常應結束於：

```text
Build succeeded.
```

Release build：

```powershell
dotnet build Astrahold.Client.csproj --configuration Release
```

這個步驟代表 C# project 編譯完成；第一次 MVP playtest 不必先 export 成 `.exe`。

---

# Part I - 把測試 CA 給 Windows Client

## 22. 複製 ca.crt

在 WSL 將 public CA certificate 複製到 Windows，例如：

```bash
cp "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.crt" \
  /mnt/c/Users/YOUR_WINDOWS_USERNAME/astrahold-ca.crt
```

例如：

```bash
cp "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.crt" \
  /mnt/c/Users/li41/astrahold-ca.crt
```

不要複製 `ca.key` 或 `server.key`。

---

# Part J - 設定 Windows Client 連 WSL

## 23. 設定 Client environment

以下假設 WSL IP 是：

```text
172.28.112.45
```

請換成 `wsl hostname -I` 的實際 IP。

PowerShell：

```powershell
$env:ASTRAHOLD_SESSION_LOGIN_URL = 'https://172.28.112.45:27444'
$env:ASTRAHOLD_SERVER_HOST = '172.28.112.45'
$env:ASTRAHOLD_SERVER_PORT = '27443'
$env:ASTRAHOLD_TLS_SERVER_NAME = '172.28.112.45'
$env:ASTRAHOLD_TLS_CA = 'C:\Users\YOUR_WINDOWS_USERNAME\astrahold-ca.crt'
```

例如：

```powershell
$env:ASTRAHOLD_TLS_CA = 'C:\Users\li41\astrahold-ca.crt'
```

`ASTRAHOLD_TLS_SERVER_NAME` 必須和 certificate SAN 可以驗證的 name/IP 一致。

---

## 24. 先測 TCP/TLS ports

Windows PowerShell：

```powershell
Test-NetConnection 172.28.112.45 -Port 27444
Test-NetConnection 172.28.112.45 -Port 27443
```

兩個都應：

```text
TcpTestSucceeded : True
```

如果 False：

1. 確認 `worldd` 還在執行；
2. 確認 `ss -lntp` 有 27443 / 27444；
3. 確認 Windows / WSL firewall；
4. 確認使用的是目前 WSL IP。

---

# Part K - 啟動正常 Godot Client

## 25. 直接執行 Main.tscn

在 Windows Client repo root：

```powershell
cd C:\src\astrahold-client
```

如果 Godot 已在 PATH：

```powershell
godot --path . res://scenes/Main.tscn
```

或使用完整路徑：

```powershell
& 'C:\Tools\Godot\4.7.1-dotnet\Godot_v4.7.1-stable_mono_win64.exe' \
  --path . \
  res://scenes/Main.tscn
```

手動 GUI 方式也可以：

1. 開 Godot 4.7.1 .NET；
2. Import `C:\src\astrahold-client\project.godot`；
3. 開啟 project；
4. Run Project。

S5-A.1 manual MVP testing 使用正常 `Main.tscn`，不要使用 diagnostic `RealServerE2E.tscn`。

---

# Part L - 第一次登入與攻城測試

## 26. Solo attacker smoke test

登入：

```text
Login: playtest-attacker
Password: 建立帳號時輸入的 attacker password
```

成功後應進入：

```text
castle-sandbox
```

Siege HUD 應顯示 attacker role。

---

## 27. 操作方式

| Input | Action |
| --- | --- |
| `W/A/S/D` 或方向鍵 | movement |
| `F` | attack `main-gate` |
| `Tab` | cycle/select remote character |
| `G` | basic attack selected/nearest character |
| `Esc` | clear combat target |

HP、damage、death、respawn、Gate HP/destruction、throne capture/contest、winner、castle ownership、next-round state 都由 Server authoritative state 決定。

---

## 28. Solo MVP acceptance

1. `playtest-attacker` 正常登入；
2. `castle-sandbox` 正常載入；
3. 移動到 `main-gate`；
4. 使用 `F` 攻擊城門；
5. Gate HP 降到 0；
6. Gate 變 breached/rubble 並可以通過；
7. 走入 throne zone；
8. 保持 uncontested 約 10 秒；
9. Server 宣告 attackers winner；
10. Client 顯示 `VICTORY`；
11. castle ownership 變成 `attackers`；
12. 下一 round 由 Server state reset/rotate。

能完整跑完這條流程，就表示目前 Playable Siege MVP 的第一條人工驗收路徑已成立。

---

## 29. Two-Client PvP / contest test

再開第二個 PowerShell / Client process，設定相同 environment：

```powershell
$env:ASTRAHOLD_SESSION_LOGIN_URL = 'https://172.28.112.45:27444'
$env:ASTRAHOLD_SERVER_HOST = '172.28.112.45'
$env:ASTRAHOLD_SERVER_PORT = '27443'
$env:ASTRAHOLD_TLS_SERVER_NAME = '172.28.112.45'
$env:ASTRAHOLD_TLS_CA = 'C:\Users\YOUR_WINDOWS_USERNAME\astrahold-ca.crt'

godot --path . res://scenes/Main.tscn
```

第二個 Client 登入：

```text
playtest-defender
```

驗收：

1. Attacker / Defender 都能看到對方；
2. `Tab` 選人；
3. `G` PvP；
4. damage / defeated / respawn 正常；
5. attacker 破門；
6. attacker + defender 同時進 throne zone 時 capture 被 contest；
7. defender 離開或被擊敗後 attacker 完成 capture；
8. attacker 顯示 `VICTORY`；
9. defender 顯示 `DEFEAT`；
10. 兩邊 winner / owner 一致；
11. next round Gate/ownership/role rotation 一致。

---

# Part M - 常見問題

## 30. Login 成功，但完全不能移動

最優先檢查 realtime UDP。

Astrahold 的可靠 bootstrap 使用 TCP/TLS，但 movement / realtime snapshot path 使用 UDP。

WSL：

```bash
ss -lnup | grep 27778
```

應看到 UDP `27778`。

確認 Server 啟動參數是：

```text
-udp 0.0.0.0:27778
```

如果 login UI 正常、世界 bootstrap 成功，但 movement 不動，通常代表 TCP/TLS path 正常而 UDP path 有問題。

---

## 31. TLS certificate validation failed

確認：

```powershell
$env:ASTRAHOLD_TLS_SERVER_NAME
$env:ASTRAHOLD_TLS_CA
```

檢查 certificate SAN：

```bash
openssl x509 \
  -in "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -noout -ext subjectAltName
```

如果 WSL IP 已改變，而 certificate 沒有新 IP，重新執行 TLS certificate 產生流程。

---

## 32. Windows 連不到 27443 / 27444

確認 WSL IP：

```powershell
wsl hostname -I
```

確認 port：

```powershell
Test-NetConnection WSL_IP -Port 27444
Test-NetConnection WSL_IP -Port 27443
```

WSL：

```bash
ss -lntp | grep -E '27443|27444'
```

如果 `worldd` 只 bind `127.0.0.1`，Windows 使用 WSL IP 時不會連到它。跨 Windows/WSL playtest 請使用本文件的 `0.0.0.0:27443` / `0.0.0.0:27444`。

---

## 33. WSL IP 每次重開都不一樣

取得新 IP：

```powershell
wsl hostname -I
```

更新 Client environment。

如果 certificate SAN 不包含新 IP，也要重新簽發 local test certificate。

如果日後要固定簡化 Windows/WSL networking，可以再評估 Windows 11 WSL mirrored networking；這不是 S5-A.1 MVP 必要條件。

---

## 34. Client C# build 失敗

確認：

```powershell
dotnet --list-sdks
```

需要 .NET SDK 8.x。

確認使用 Godot 4.7.1 .NET，不是 standard edition。

重新：

```powershell
dotnet clean Astrahold.Client.csproj
dotnet restore Astrahold.Client.csproj
dotnet build Astrahold.Client.csproj --configuration Debug
```

---

## 35. Server Go build 失敗

確認：

```bash
go version
cat go.mod | head
```

目前 Server baseline 是 Go 1.26。

重新下載 modules：

```bash
go clean -modcache
go mod download
go build -o .tools/playtest/worldd ./cmd/worldd
go build -o .tools/playtest/accountctl ./cmd/accountctl
```

---

# Part N - Optional Windows EXE Export

## 36. `dotnet build` 和 Windows `.exe` 的差別

```powershell
dotnet build Astrahold.Client.csproj
```

會編譯 C# project，但不是 Godot Windows export packaging。

第一次 S5-A.1 manual playtest 直接用：

```powershell
godot --path . res://scenes/Main.tscn
```

即可。

如果要產生可以直接執行的 Windows package，例如：

```text
Astrahold.exe
```

需要 Godot Export Templates + Windows Desktop export preset。

目前 repository 不把 Windows export preset 當 S5-A.1 MVP 必要條件，因此第一輪測試先把 **Server → login → game bootstrap → UDP movement → siege → Victory** 跑通，再處理 packaging。

Godot GUI 中可使用：

```text
Editor
→ Manage Export Templates
→ 安裝 4.7.1 templates
→ Project
→ Export
→ Add Windows Desktop
→ Export Project
```

如果後續要做正式可下載的 Windows build，應另行把 `export_presets.cfg`、build output layout、version metadata 與 release pipeline 固定下來。

---

# Part O - 最短每日測試流程

安裝完成後，之後每天通常只需要以下步驟。

## WSL

```bash
cd ~/src/astrahold-server
git pull --ff-only

go build -o .tools/playtest/worldd ./cmd/worldd
go build -o .tools/playtest/accountctl ./cmd/accountctl

export ASTRAHOLD_PLAYTEST_ROOT="$PWD/.playtest/siege"
export WSL_IP=$(hostname -I | awk '{print $1}')

.tools/playtest/worldd \
  -tcp 127.0.0.1:27777 \
  -udp 0.0.0.0:27778 \
  -siege-match config/siege-match-playtest.json \
  -session-login-account-file "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  -session-login-tls-listen 0.0.0.0:27444 \
  -session-login-tls-cert "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -session-login-tls-key "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key" \
  -trusted-tls-listen 0.0.0.0:27443 \
  -trusted-tls-cert "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -trusted-tls-key "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key" \
  -character-state-dir "$ASTRAHOLD_PLAYTEST_ROOT/character-state" \
  -character-state-save-journal "$ASTRAHOLD_PLAYTEST_ROOT/character-state-saves.journal" \
  -character-state-save-checkpoint "$ASTRAHOLD_PLAYTEST_ROOT/character-state-saves.checkpoint.json" \
  -death-outcome-journal "$ASTRAHOLD_PLAYTEST_ROOT/death-outcomes.journal" \
  -death-outcome-checkpoint "$ASTRAHOLD_PLAYTEST_ROOT/death-outcomes.checkpoint.json" \
  -siege-ownership-dir "$ASTRAHOLD_PLAYTEST_ROOT/siege-ownership"
```

## Windows

```powershell
cd C:\src\astrahold-client
git pull --ff-only

dotnet build Astrahold.Client.csproj --configuration Debug

$WSL_IP = '172.28.112.45' # 改成 wsl hostname -I 的目前值
$env:ASTRAHOLD_SESSION_LOGIN_URL = "https://${WSL_IP}:27444"
$env:ASTRAHOLD_SERVER_HOST = $WSL_IP
$env:ASTRAHOLD_SERVER_PORT = '27443'
$env:ASTRAHOLD_TLS_SERVER_NAME = $WSL_IP
$env:ASTRAHOLD_TLS_CA = 'C:\Users\YOUR_WINDOWS_USERNAME\astrahold-ca.crt'

godot --path . res://scenes/Main.tscn
```

---

# S5-A.1 Definition of Done

這份 setup guide 的目標不是建立完整 production deployment，而是讓 developer/operator 可以從乾淨環境完成：

```text
install WSL / toolchains
→ build worldd + accountctl
→ create pre-built playtest account
→ create local TLS
→ start production worldd
→ build normal Godot Client
→ login from Windows into WSL Server
→ activate realtime UDP
→ move in castle-sandbox
→ breach main-gate
→ capture throne
→ see VICTORY / DEFEAT
→ verify Server-owned castle ownership / next round
```

進一步的 authoritative siege contract 與 manual gameplay acceptance 參考：

- [`S5A1_PLAYABLE_SIEGE_MVP.md`](S5A1_PLAYABLE_SIEGE_MVP.md)

Public registration、MFA、distributed account DB、multi-server failover、F.29 supervisor handoff、final art、inventory/guild/quest 等都不是這份 S5-A.1 安裝/編譯手冊要解決的問題。
