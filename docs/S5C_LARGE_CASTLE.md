# S5-C Large Castle Vertical Slice

`castle-sandbox` is now a large-castle vertical slice rather than the previous small siege test
box.

## Shared world contract

```text
world_id: castle-sandbox
revision: s5c-large-castle-001
field: 300m x 340m
surfaces: 15
portals: 4
blockers: 33
gates: 1 (main-gate)
```

The authoritative route is:

```text
field-camp (-145m)
  -> long approach road
  -> main-gate (z 9.5)
  -> outer courtyard
  -> inner city
  -> keep entrance
  -> throne approach (z 90..105)
```

The west and east outer-wall ramps transition from ground to wall-walk through dedicated landing
surfaces. The corner towers no longer overlap the ramp entry, so the visual route and the
authoritative navigation route agree.

The major courtyard barracks, inner-market buildings and rear throne hall are Server-owned
movement/LOS blockers. The central processional route remains open for the siege loop.

## Client presentation

The Client binds `visual.json` to the same gameplay revision and renders a presentation-only large
castle: moat, drawbridge, gatehouse, curtain-wall crenellations, bastions, inner gatehouse,
processional roads, standards, lanterns and keep architecture. These meshes do not add Client
collision, navigation or LOS authority.

```text
visual_revision: s5c-large-castle-visual-001
presentation_mode: large-castle-v1
```

Changing the castle art must not change the Server gameplay contract unless the shared
`gameplay.json` revision is intentionally advanced and both packages are synchronized.
