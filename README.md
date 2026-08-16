# Tecmo Sooper Bowl

**Status: in development** — playable prototype, balance and features still moving fast.

A **Tecmo-inspired** arcade football game written in Go: short playbooks, a live snap loop, stamina, jukes, and defenses that start to **catch your tendencies**.

> Not affiliated with Tecmo, Nintendo, or the NFL. “Sooper” is intentional.

![Tecmo Sooper Bowl — pre-snap look (HUD hides the call)](docs/presnap-look.png)

Read the defense before you snap: one high vs two high, corners off vs pressed. Sometimes the picture is a lie — they rotate after the snap. **D** names the call (`look → live` when they disguise).

## Quick start

```bash
git clone https://github.com/randerson1184/tecmo-sooper-bowl.git
cd tecmo-sooper-bowl
go run ./cmd/game
```

Requires **Go 1.26.5+** (see `go.mod`). On macOS, Xcode Command Line Tools are usually enough for Ebitengine.

Browser (local):

```bash
./web/serve.sh
```

Then open **http://127.0.0.1:8080/**. Desktop keyboard only. Click the field so keys stay in the game. After 20–30 snaps, **Download anonymous play film** (JSONL — gameplay only, no name/email/keys). Desktop `go run` still writes `logs/*.jsonl`.

**Public link (free):** GitHub Pages. After this repo has Pages enabled (Settings → Pages → Source: GitHub Actions), a push to `main` builds `web/game.wasm` and publishes. Expected URL: `https://randerson1184.github.io/tecmo-sooper-bowl/`. Custom domain is optional (~$10–15/year at a registrar; Pages itself stays free).

**Who is actually playing** (not page views): a first-snap ping to a tiny Cloudflare Worker. Page loads do not count — only sessions that complete a snap. One-time setup (free):

```bash
cd web/playcount
npx wrangler login
npx wrangler kv namespace create PLAYS
# paste the id into wrangler.toml, then:
npx wrangler deploy
```

Put the printed `*.workers.dev` URL in `web/config.js` as `TECMO_PLAYCOUNT_URL` and push. Open that same URL in a browser to see `{sessions_that_snapped, snaps_recorded}`.

## Controls

| Key | Action |
|-----|--------|
| **1–4** | Select slot: **1** inside · **2** outside · **3** quick (slant) · **4** shot (post) |
| **Shift+3** | Cycle quick game (**hitch**) — the HUD name is what snaps |
| **Shift+4** | Cycle shot: **PA Post** · **PA Glance** (fake inside zone; glance sits over the Mike) |
| **Space** | Snap · on passes, **throw** to primary (green ring). On PA, Space during the mesh **buffers** until the fake finishes |
| **↑ ↓ ← →** | Steer QB / ball carrier (**↑** = toward their end zone) |
| **Shift** / **E** | Juke (burst + short tackle evade). On PA during the mesh, **aborts the fake** (no bite, no juke) |
| **T** | Play-log summary (terminal) |
| **D** | Toggle named defensive call (hidden by default). Disguise prints `look → live` |
| **R** | Reset drive |
| **Esc** | Quit |

## What’s in already

- Full-field camera that zooms on the ball
- Four slots: inside zone · sweep · quick (slant / hitch) · shot (**post**)
- O-line push, pursuit, tackles, sacks, incompletes, TDs
- Stamina + juke
- Tendency-aware **front + coverage shell** (hitch diet → Cover 2; run success → Run Fit; pass diet lights the box only if the run is not already working)
- Pre-snap **looks** instead of a named call: one-high off (Cover 3), one-high press (Man Free), two-high squat (Cover 2). Staff may **disguise** — same picture, different coverage after the snap. **D** shows `look → live` when they lie
- Offensive line: 1:1 pass pro, pocket collapses if you hold it; successful runs buy a beat, 3rd/4th & long gets hotter
- Every front sets a sweep edge (light boxes later/wider, not vacant)
- JSONL play logging under `logs/` (`thrown`, `carrier`, `qb_keep`, `keep_threat`, `run_threat`, PA `mesh` / `leftover_sec` / `release_at`, plus `look` / `disguised`)

## Plays

The HUD name is what snaps. Situation changes the defense, not your button.

1. **Inside Zone** (1) — up the gut
2. **Toss Sweep** (2) — stretch right
3. **Slant** (3) — quick timing throw to the green-ring primary
4. **Hitch** (Shift+3) — outside stop + YAC
5. **Post** (4) — intermediate shot, ~16-yard break
6. **PA Post** (Shift+4) — mesh with the RB, then throw the post. Space during the fake **buffers**; Shift **aborts** (no bite). A working run buys a leftover window after the mesh; pass-sell only shaves that window while the run is live. Holding past it costs rush
7. **PA Glance** (Shift+4 twice) — same fake; leftover sit behind the Mike. Fake, then throw when he sits if **PA WINDOW** is still up (~0.25–0.70s after a live run). Miss it and he sits in traffic — not a delayed post

## Stack

| Piece | Choice |
|-------|--------|
| Language | Go |
| Engine | [Ebitengine](https://ebitengine.org/) |
| Why | Fast 2D, single binaries, later WASM playtests |

Design notes and phases: **[DESIGN.md](DESIGN.md)**

## Project layout

```
cmd/game/           window + game loop
internal/field/     yard geometry
internal/game/      score, down, distance
internal/playbook/  plays + defense calls
internal/sim/       snap loop, blocks, tackle, pass
internal/ai/        tendencies + ChooseDefense
internal/render/    camera + draw
internal/logplay/   session play logs
docs/               screenshots (presnap-look.png is the current HUD)
```

## Dev

```bash
go test ./...
go build -o tecmo-sooper-bowl ./cmd/game
```

## Roadmap (rough)

- [x] Playable snap loop
- [x] Camera, stamina, juke
- [x] Run blocking + play balance pass
- [x] Play logging
- [x] Coverage shells + a real post on 4
- [x] Readable pre-snap looks (hide the named defensive call; **D** to name it)
- [ ] Ratings / broken tackles
- [ ] Season shell
- [x] Local WASM playtest (`./web/serve.sh`) + anonymous film download
- [ ] GitHub Pages public link (enable Actions source in repo settings)

## License

MIT — see [LICENSE](LICENSE).

## Credits

Fan-inspired original. Built for fun and for learning Go game systems.
