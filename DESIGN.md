# Tecmo Sooper Bowl — Design Document

> Working title on purpose. Tecmo-inspired American football with a smarter defensive brain — not a byte-accurate NES rom.

**Status:** Phase 0 / MVP skeleton  
**Repo:** local `Tecmo_Sooper_Bowl` (publish when ready)  
**Last updated:** 2026-08-12

---

## 1. Elevator pitch

A **fast, readable, arcade** football game that *feels* like Tecmo Super Bowl: short playbooks, star power, big moments — plus **defenses that learn your tendencies** so the same cheese run does not work forever.

**Not** Madden. **Not** a pixel-perfect Tecmo clone. **Yes** Tecmo *soul* + modern CPU awareness.

---

## 2. Pillars (non-negotiable)

1. **Readable in two seconds** — formation, ball, sticks, down/distance are obvious.
2. **Arcade, not sim** — small playbooks, exaggerated athleticism, decisive outcomes.
3. **Stars matter** — ratings create identity (elite RB vs elite CB feels different).
4. **CPU has a plan** — tendencies → game plans → fewer free blown coverages.
5. **Always playable** — every milestone ends in “I can run a drive.”

## 3. Non-goals (for a long time)

- Pixel-perfect NES sprites / frame-accurate Tecmo mechanics  
- Full modern NFL playbooks and playbooks from real film  
- Online multiplayer (maybe much later)  
- Machine-learning models inside the game  
- Photoreal graphics  

---

## 4. Language & stack

### Decision: **Go + [Ebitengine](https://ebitengine.org/)**

| Criterion | Why Go + Ebitengine wins here |
|-----------|-------------------------------|
| **Performance** | Plenty for 22 stick-figure athletes + 2D field; 60 FPS is easy |
| **Portability** | Cross-compile to **macOS / Windows / Linux** single binaries |
| **Share / playtest** | Native builds *and* **WebAssembly** browser builds (link to play) |
| **Vibe-coding fit** | Same language as `snake-cli`; small surface area; great tooling |
| **Distribution** | `go build` artifacts, GitHub Releases, optional itch.io / static host for WASM |

### Alternatives considered

| Stack | Pros | Cons | Verdict |
|-------|------|------|---------|
| **TypeScript + Canvas** | Zero-install playtest in browser | Heavier “game feel” plumbing; packaging later is messier | Great for a pure web jam; secondary if we need max frictionless testing first |
| **Rust + macroquad/bevy** | Max perf, great WASM story | Steeper for iteration speed while designing feel | Overkill for v1 |
| **Godot** | Fast content iteration, exporters | Engine-shaped repo; less “from scratch systems” learning | Fine if art-heavy later; not our start |
| **Python + pygame** | Easy prototypes | Packaging & “send to friend” friction | Skip for shipping |

**Portable playtest path**

1. Dev: `go run ./cmd/game` on your Mac  
2. Friends (native): attach platform binaries on a Release  
3. Friends (easiest): `GOOS=js GOARCH=wasm` build → static page (GitHub Pages)

---

## 5. Feel targets (Tecmo soul)

| Feel | Design choice |
|------|----------------|
| Play call | Pre-snap menu: few formations × few plays (not 500 concepts) |
| Control | User steers ball carrier / QB timing window; rest is assignment AI |
| Pace | Snappy downs; minimal sim simming between plays |
| Stamina | Heavy usage of one skill player degrades burst/success (phase 2) |
| Juice | Big play feedback (phase 2+): screen flash, text pop, simple SFX |

---

## 6. Adaptive defense (the differentiator)

**No neural nets.** Explicit, tunable systems.

### 6.1 Tendency tracker

Bucket recent offensive plays (e.g. last 12–20):

- Play type: run / pass  
- Side: left / middle / right  
- Depth: short / intermediate / deep (passes)  
- Situation: down + distance class (e.g. short yardage, 3rd & long, red zone)

### 6.2 Game plans (CPU picks one)

Small finite set, e.g.:

| Plan | Intent |
|------|--------|
| `Base` | Balanced |
| `RunFit` | Extra box defender, tighter run fits |
| `PassRush` | More pressure, softer underneath |
| `SoftZone` | Protect deep, give up underneath |
| `Blitz` | Risk for disruption on obvious pass / long yardage |

Selection = **situation priors** + **tendency weights** + **noise** (so it is not psychic).

### 6.3 Coverage rules (“no free blown coverages”)

Hard assignment constraints before fancy AI:

- Someone always owns **deep help** unless blitzing all-out  
- Outside leverage / force direction on run  
- Man: trail rules; Zone: landmark + receiver in zone  
- Never leave the offense’s #1 free *and* deep with no help without an intentional blitz call  

Blown coverages become **rare failures of tradeoffs**, not random forgetfulness.

### 6.4 Player-facing feedback (later)

Optional coach tip / sideline mutter: *“They’re loading the box…”* so adaptation is readable.

---

## 7. Architecture

```
Tecmo_Sooper_Bowl/
├── DESIGN.md                 # this file
├── README.md
├── go.mod
├── cmd/
│   └── game/                 # entrypoint (window, loop)
│       └── main.go
├── internal/
│   ├── field/                # geometry, yards, hash marks, bounds
│   ├── game/                 # match state: down, distance, clock, score
│   ├── playbook/             # formations, routes, blocking assignments
│   ├── sim/                  # movement, collision, tackle, catch, throw
│   ├── ai/                   # pursuit, coverage, play calling, tendencies
│   ├── ratings/              # player attributes (phase 2)
│   └── render/               # draw field, players, UI (Ebitengine)
└── web/                      # wasm glue / index.html (when we ship browser)
```

**Rule:** `sim` and `ai` must be testable **without** a window (pure logic). Rendering is a thin view over state.

### Core loop (one play)

```
PreSnap → SelectPlay → Snap → ResolvePlay (ticks) → DeadBall → UpdateTendencies → NextDown / Score
```

---

## 8. Phased roadmap

### Phase 0 — Skeleton

- [x] DESIGN.md  
- [x] Module + package layout  
- [x] Window + field draw + placeholder units  
- [x] Stub play state (down/distance/score)  
- [x] Stub tendency + game-plan types  

### Phase 1 — Vertical slice (current)

**Exit criteria:** one full **drive** is fun with ugly graphics.

- [x] 11v11 placeholders on a 100-yard field  
- [x] 4 offensive plays (Inside Zone, Sweep, Slant, Hitch)  
- [x] Defense calls affect pursuit / coverage (Base, RunFit, SoftZone, PassRush, Blitz)  
- [x] Snap → handoff / pass → control RB or QB → tackle / OOB / sack / incomplete / TD  
- [x] First downs, turnovers on downs, re-spot after score at own 25  
- [ ] Juice / camera follow (optional)

### Phase 2 — Feel (current)

- [x] Camera follow + zoom on the ball  
- [x] Larger player sprites with role labels  
- [x] Stamina / fatigue on featured ball carriers (RB/QB/WR)  
- [x] Juke / spin (Shift) with cooldown + evade window  
- [ ] Basic ratings sheet (speed, power, hands, coverage)  
- [ ] Better tackle angles / broken tackles from power  
- [ ] Play-select UX closer to Tecmo cadence

### Phase 3 — Brain

- [ ] Live tendency tracker wired into CPU play calling  
- [ ] Coverage assignment rules (no free deep bombs)  
- [ ] Situational blitz logic  
- [ ] Readable feedback that defense is adjusting  

### Phase 4 — Content & juice

- [ ] Team/roster data files  
- [ ] Season shell (schedule, standings)  
- [ ] SFX / simple presentation  
- [ ] WASM build + static host for public playtests  
- [ ] Balance pass with recorded “cheese” scenarios as regression tests  

---

## 9. MVP vertical slice (definition of done)

You can:

1. Launch the game  
2. Pick from a tiny playbook  
3. Run or pass once per play  
4. Move the chains or score  
5. Face a CPU that at least **calls different defenses** (even if dumb)  
6. Quit cleanly  

Graphics may be rectangles. Fun > fidelity.

---

## 10. Testing strategy

| Layer | How |
|-------|-----|
| Field math / down logic | `go test` pure functions |
| Tendency → plan selection | Table-driven tests with scripted play histories |
| Coverage invariants | “After assignment, deep third has owner” assertions |
| Feel | Human playtests; record cheese clips as future fixtures |

---

## 11. Open questions

- 11v11 from day one vs 7v7 until feel is right? **Lean 11v11 placeholders, simplified roles.**  
- Camera: full field always vs sideline scroll? **MVP: fit full field in window.**  
- Two-player hotseat? **After Phase 1.**  
- Real NFL names/teams? **No — fictional or generic until legal comfort; “Sooper” is a feature.**  

---

## 12. Success metrics (qualitative)

- A friend can score a TD without a tutorial longer than 30 seconds  
- Running the same play 8 times in a row **gets harder**, not identical  
- Deep shots are earned (mismatch, blitz punishment), not free  
- You still say “one more drive” at 1 a.m.  

---

## 13. License / credit (intent)

- Original code: project author’s (MIT likely, same as snake-cli)  
- Tecmo / NFL are trademarks of their owners — this is a **fan-inspired original**, not affiliated  

---

*Next concrete coding step after skeleton: Phase 1 play loop — snap, run play, tackle, dead ball.*
