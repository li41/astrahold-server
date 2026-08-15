# S4-E.1 — True Go Server ↔ Godot Multi-Client E2E Server Harness

## Scope

S4-E.1 adds a CI-only Go server executable used by the paired Godot Client end-to-end workflow. The goal is to cross the actual process, TCP, UDP, Protocol v8, WorldRuntime, combat, Siege, respawn, and durable castle-ownership boundaries instead of relying only on the existing in-process C# loopback fixture.

The harness is deliberately **not** `cmd/worldd` and does not add a development login or authentication bypass to the production binary.

## Production packages under test

`cmd/e2eserver` composes the same authoritative packages used by `worldd`:

- `tcpudp` transport with real TCP Reliable and UDP Realtime sockets;
- GameV1 / Protocol v8;
- Gameplay World loading and navigation;
- WorldRuntime single-owner loop;
- combat action catalog;
- Siege gate, trusted participant assignment, throne capture, resolution, and round reset;
- respawn policy;
- `siegeownership.Store` durability barrier.

No gameplay result is injected directly by the harness.

## Deterministic two-client identity

The harness accepts exactly two sessions and assigns trusted character identities by connection order:

1. session 1 → `e2e-attacker`;
2. session 2 → `e2e-defender`.

The paired Client workflow starts the attacker Godot process first and waits for its real Server connection marker before starting the defender process. A third connection is rejected.

This deterministic mapping exists only in `cmd/e2eserver`. Production `cmd/worldd` keeps its existing identity/authentication behavior unchanged.

## Network safety

The executable refuses non-loopback TCP or UDP listen addresses. It is a local CI harness, not a deployable public Server profile.

## E2E fixtures

`testdata/s4e/` retains the production gameplay geometry and combat catalog while shortening only time-based test waits:

- Siege participant roster binds the two trusted E2E character IDs;
- throne capture is 1 second instead of the production 10 seconds;
- respawn delays are 1 second instead of production 5/8/10 seconds.

The production `config/siege-match.json`, `config/respawn-policy.json`, and Gameplay World remain unchanged.

## Durable round completion marker

The harness observes the authoritative WorldRuntime after each owner step. After the clients complete round 1, D.3D schedules the normal next-round reset. The Server emits `ASTRAHOLD_E2E_SERVER_OK` only when all of these are true:

- Match is Round 2 / Gate;
- ownership-derived roles have rotated (`defenders` attack, `attackers` defend);
- in-memory castle owner is `attackers`, revision 2;
- reloading `siegeownership.Store` from disk returns exactly the same ownership state.

This makes the CI marker evidence of the real D.3B durability barrier plus D.3C/D.3D round lifecycle, not merely a Client-side inference.

## Paired Client scenario

The Client half of S4-E.1 runs two actual Godot 4.7.1 .NET headless processes and drives:

1. Protocol v8 world bootstrap and UDP activation;
2. mutual player spawn/vitals visibility;
3. authoritative PvP defeat and respawn;
4. movement to the main gate and ten real `basic-attack` actions;
5. authoritative Gate destroyed/blocker disabled and Throne phase;
6. movement through the breached gate into the authoritative throne zone;
7. attacker throne capture, durable ownership transfer, and Completed result;
8. automatic Round 2 reset, gate restoration, and ownership-derived role rotation.

## Preserved boundaries

S4-E.1 does not change:

- Protocol v8 or Message 106 fields;
- GameV1 realtime binary layouts;
- UDP MTU 1200;
- Gameplay World schema 2 / `s3d-001`;
- production Siege config schema/revision/capture duration;
- production respawn policy;
- `cmd/worldd` authentication or persistence configuration;
- lifecycle/Vitals/scaling budgets.
