# S5-A.1 - Playable Siege MVP / Local Product Playtest Profile

S5-A.1 turns the already-proven `castle-sandbox` siege vertical slice into a local product playtest path. It does not add a new siege ruleset, public registration, remote process control, or Client authority.

The target MVP loop is:

```text
pre-created account
  -> normal Godot Main.tscn login
  -> production worldd
  -> castle-sandbox
  -> attacker/defender role
  -> PvP / respawn
  -> breach main-gate
  -> enter throne zone
  -> authoritative capture / contest
  -> victory or defeat
  -> durable castle ownership
  -> next round
```

## Playtest identities

`config/siege-match-playtest.json` is a schema-v3 siege match profile with two trusted CharacterIDs:

| Login / CharacterID | Authoritative siege team |
| --- | --- |
| `playtest-attacker` | attacker |
| `playtest-defender` | defender |

The account store remains the source of the Server-owned CharacterID. The Client never submits or chooses its siege team. An account whose CharacterID is not in the playtest profile is admitted normally but is not a configured siege participant.

For a solo smoke test, only the attacker needs to be online. Start the defender in a second normal Client when testing PvP, contest, death/respawn, and Victory/Defeat presentation from both sides.

## 1. Build production tools

From the `astrahold-server` repository root:

```bash
mkdir -p .tools/playtest
go build -o .tools/playtest/worldd ./cmd/worldd
go build -o .tools/playtest/accountctl ./cmd/accountctl
```

## 2. Create a local playtest data root

The following example is intended for a local or otherwise controlled test machine. It creates a local CA only for this playtest. Do not reuse the generated CA/private key as a production PKI.

```bash
export ASTRAHOLD_PLAYTEST_ROOT="$PWD/.playtest/siege"
rm -rf "$ASTRAHOLD_PLAYTEST_ROOT"
mkdir -p "$ASTRAHOLD_PLAYTEST_ROOT/tls" \
  "$ASTRAHOLD_PLAYTEST_ROOT/character-state"

.tools/playtest/accountctl init \
  -path "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json"
```

Create the attacker account without placing its password on a command line:

```bash
read -r -s -p 'Attacker password: ' ATTACKER_PASSWORD; printf '\n'
printf '%s\n' "$ATTACKER_PASSWORD" | .tools/playtest/accountctl create \
  -path "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  -login playtest-attacker \
  -character playtest-attacker \
  -password-stdin
unset ATTACKER_PASSWORD
```

For a two-Client test, also create the defender:

```bash
read -r -s -p 'Defender password: ' DEFENDER_PASSWORD; printf '\n'
printf '%s\n' "$DEFENDER_PASSWORD" | .tools/playtest/accountctl create \
  -path "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  -login playtest-defender \
  -character playtest-defender \
  -password-stdin
unset DEFENDER_PASSWORD
```

## 3. Create local TLS material

```bash
openssl req -x509 -newkey rsa:2048 -nodes \
  -keyout "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.key" \
  -out "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.crt" \
  -days 7 -sha256 \
  -subj '/CN=Astrahold Local Siege Playtest CA' \
  -addext 'basicConstraints=critical,CA:TRUE' \
  -addext 'keyUsage=critical,keyCertSign,cRLSign'

openssl req -newkey rsa:2048 -nodes \
  -keyout "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key" \
  -out "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.csr" \
  -subj '/CN=localhost'

cat >"$ASTRAHOLD_PLAYTEST_ROOT/tls/server.ext" <<'EOF'
subjectAltName=DNS:localhost,IP:127.0.0.1
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
EOF

openssl x509 -req \
  -in "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.csr" \
  -CA "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.crt" \
  -CAkey "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.key" \
  -CAcreateserial \
  -out "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -days 7 -sha256 \
  -extfile "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.ext"

chmod 600 \
  "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  "$ASTRAHOLD_PLAYTEST_ROOT/tls/ca.key" \
  "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key"
```

Only `ca.crt` is needed by the Client. Never copy `ca.key` or `server.key` to the Client machine.

## 4. Start production `worldd`

Run this from the Server repository root so the normal world/combat/respawn configs resolve from the repository:

```bash
.tools/playtest/worldd \
  -tcp 127.0.0.1:27777 \
  -udp 127.0.0.1:27778 \
  -siege-match config/siege-match-playtest.json \
  -session-login-account-file "$ASTRAHOLD_PLAYTEST_ROOT/accounts.json" \
  -session-login-tls-listen 127.0.0.1:27444 \
  -session-login-tls-cert "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -session-login-tls-key "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key" \
  -trusted-tls-listen 127.0.0.1:27443 \
  -trusted-tls-cert "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.crt" \
  -trusted-tls-key "$ASTRAHOLD_PLAYTEST_ROOT/tls/server.key" \
  -character-state-dir "$ASTRAHOLD_PLAYTEST_ROOT/character-state" \
  -character-state-save-journal "$ASTRAHOLD_PLAYTEST_ROOT/character-state-saves.journal" \
  -character-state-save-checkpoint "$ASTRAHOLD_PLAYTEST_ROOT/character-state-saves.checkpoint.json" \
  -death-outcome-journal "$ASTRAHOLD_PLAYTEST_ROOT/death-outcomes.journal" \
  -death-outcome-checkpoint "$ASTRAHOLD_PLAYTEST_ROOT/death-outcomes.checkpoint.json" \
  -siege-ownership-dir "$ASTRAHOLD_PLAYTEST_ROOT/siege-ownership"
```

A healthy startup includes all of these facts in the logs:

```text
Astrahold worldd ready: protocol=9
siege_match_revision=s5a1-playtest-001
siege_match=castle-sandbox-siege-playtest
breach_gate=main-gate throne=throne
session login issuance: enabled=true
trusted TLS ingress: enabled=true
```

The login/game TLS listeners above are loopback-only. When testing across machines, bind deliberate LAN addresses, issue a certificate containing the actual hostname/IP, and adjust Client settings accordingly. Do not expose the raw development TCP/UDP backends directly to the Internet.

## 5. Start the normal Godot Client

Use the current `astrahold-client` and its normal `Main.tscn`; do not use the diagnostic `RealServerE2E.tscn` scene for manual MVP testing.

For a Client running on the same machine, configure:

```text
ASTRAHOLD_SESSION_LOGIN_URL=https://localhost:27444
ASTRAHOLD_SERVER_HOST=127.0.0.1
ASTRAHOLD_SERVER_PORT=27443
ASTRAHOLD_TLS_SERVER_NAME=localhost
ASTRAHOLD_TLS_CA=<path to ca.crt>
```

PowerShell example:

```powershell
$env:ASTRAHOLD_SESSION_LOGIN_URL = 'https://localhost:27444'
$env:ASTRAHOLD_SERVER_HOST = '127.0.0.1'
$env:ASTRAHOLD_SERVER_PORT = '27443'
$env:ASTRAHOLD_TLS_SERVER_NAME = 'localhost'
$env:ASTRAHOLD_TLS_CA = 'C:\path\to\ca.crt'
dotnet build Astrahold.Client.csproj --configuration Debug
godot --path . res://scenes/Main.tscn
```

Login through the normal product login UI as `playtest-attacker` with the password created above. Open a second Client and login as `playtest-defender` when needed.

## 6. Current playable controls

| Input | Action |
| --- | --- |
| `W/A/S/D` or arrow keys | authoritative movement intent |
| `F` | attack the authoritative `main-gate` objective |
| `Tab` | cycle/select a remote character in AOI |
| `G` | basic attack the selected/nearest remote character |
| `Esc` | clear the current combat target |

Damage, death, respawn, Gate HP/destruction, throne presence/capture/contest, winner, ownership, and next-round state remain Server authoritative.

## 7. MVP manual acceptance checklist

### Solo attacker smoke test

1. Login as `playtest-attacker` through normal `Main.tscn`.
2. Verify `castle-sandbox` loads and Siege HUD shows the attacker role.
3. Move to `main-gate`.
4. Press `F` as needed until authoritative Gate HP reaches zero and the Gate becomes breached/rubble.
5. Move through the now-open Gate into the throne zone around X -4..4 / Z 27..33.
6. Stay in the zone for the configured 10-second uncontested capture duration.
7. Verify the Server declares attackers the winner, transfers castle ownership to `attackers`, and the Client shows `VICTORY`.
8. Verify the next round is scheduled/reset by authoritative Server state rather than Client mutation.

### Two-Client siege test

1. Start attacker and defender using the two pre-created accounts.
2. Verify both characters are visible through normal spawn/AOI replication.
3. Use `Tab` then `G` to exercise PvP; verify damage, defeated state, and authoritative respawn.
4. Breach `main-gate` as attacker.
5. Place attacker and defender together in the throne zone and verify capture is contested rather than Client-decided.
6. Remove/defeat the defender and complete the capture.
7. Verify attacker sees `VICTORY`, defender sees `DEFEAT`, and both agree on the same winner/owner.
8. Verify next-round Gate restoration and ownership-derived role rotation.

## Non-goals

S5-A.1 deliberately does not add:

- public account registration;
- Client-chosen team/role;
- bots or fake defenders;
- Client-side HP, Gate, capture, winner, ownership, or round mutation;
- a second test-only siege ruleset;
- Protocol v10, DTLS, QUIC, refresh tokens, or remembered sessions;
- F.29 remote SIGHUP/restart/supervisor execution;
- final art, inventory, guilds, quests, or a multi-map world.

The purpose of S5-A.1 is narrower: make the existing authoritative siege vertical slice directly reachable through production `worldd`, pre-created accounts, and normal `Main.tscn` so the MVP can be manually played and evaluated.
