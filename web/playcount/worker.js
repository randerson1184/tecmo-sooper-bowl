// Anonymous play counter + film intake.
// POST /              {session_id, build, snaps}
// POST /film          raw JSONL (session line + snaps)
// GET  /              {sessions_that_snapped, snaps_recorded, films}
// GET  /films         recent film metadata
// GET  /film/:id      raw JSONL

const MAX_SNAPS = 10000;
const MAX_FILM = 1_500_000;
const TTL = 60 * 60 * 24 * 90;

function corsHeaders(extra) {
  return {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
    ...(extra || {}),
  };
}

function json(data, status) {
  return new Response(JSON.stringify(data), {
    status: status || 200,
    headers: corsHeaders({ "Content-Type": "application/json" }),
  });
}

function cleanSessionID(v) {
  return String(v || "")
    .replace(/[^a-fA-F0-9-]/g, "")
    .slice(0, 32);
}

async function readStats(env) {
  return JSON.parse((await env.PLAYS.get("stats")) || '{"sessions":0,"snaps":0,"films":0}');
}

async function writeStats(env, stats) {
  await env.PLAYS.put("stats", JSON.stringify(stats));
}

async function bumpSession(env, sessionID, snaps, build) {
  const key = "s:" + sessionID;
  const prev = JSON.parse((await env.PLAYS.get(key)) || "null");
  const now = Date.now();
  if (prev && now - (prev.last || 0) < 1500 && snaps <= (prev.snaps || 0)) {
    return { ok: true, throttled: true, stats: await readStats(env) };
  }
  const rec = {
    session_id: sessionID,
    build: String(build || "")
      .replace(/[^a-zA-Z0-9._-]/g, "")
      .slice(0, 40),
    snaps: Math.max(snaps, prev ? prev.snaps : 0),
    first: prev ? prev.first : now,
    last: now,
  };
  await env.PLAYS.put(key, JSON.stringify(rec), { expirationTtl: TTL });
  const stats = await readStats(env);
  if (!prev) {
    stats.sessions = (stats.sessions || 0) + 1;
  }
  stats.snaps = (stats.snaps || 0) + Math.max(0, rec.snaps - (prev ? prev.snaps : 0));
  await writeStats(env, stats);
  return { ok: true, sessions: stats.sessions, snaps: stats.snaps };
}

export default {
  async fetch(req, env) {
    if (req.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders() });
    }

    const url = new URL(req.url);
    const path = url.pathname.replace(/\/+$/, "") || "/";

    if (req.method === "GET" && path === "/") {
      const stats = await readStats(env);
      return json({
        sessions_that_snapped: stats.sessions || 0,
        snaps_recorded: stats.snaps || 0,
        films: stats.films || 0,
        note: "A session counts when the first snap is recorded — not a page view.",
      });
    }

    if (req.method === "GET" && path === "/films") {
      const listed = await env.PLAYS.list({ prefix: "f:", limit: 50 });
      const films = [];
      for (const k of listed.keys) {
        const meta = JSON.parse((await env.PLAYS.get("m:" + k.name.slice(2))) || "null");
        films.push(
          meta || {
            session_id: k.name.slice(2),
          }
        );
      }
      films.sort((a, b) => (b.saved || 0) - (a.saved || 0));
      return json({ films, count: films.length });
    }

    if (req.method === "GET" && path.startsWith("/film/")) {
      const id = cleanSessionID(path.slice("/film/".length));
      if (!id) {
        return json({ error: "id" }, 400);
      }
      const body = await env.PLAYS.get("f:" + id);
      if (!body) {
        return json({ error: "missing" }, 404);
      }
      return new Response(body, {
        status: 200,
        headers: corsHeaders({
          "Content-Type": "application/x-ndjson; charset=utf-8",
        }),
      });
    }

    if (req.method === "POST" && path === "/film") {
      const text = await req.text();
      if (!text || text.length > MAX_FILM) {
        return json({ error: "size" }, 400);
      }
      let meta;
      try {
        meta = JSON.parse(text.split("\n", 1)[0]);
      } catch {
        return json({ error: "jsonl" }, 400);
      }
      if (meta.type !== "session") {
        return json({ error: "envelope" }, 400);
      }
      const sessionID = cleanSessionID(meta.session_id);
      const snaps = Math.floor(Number(meta.snaps) || 0);
      if (!sessionID || sessionID.length < 8) {
        return json({ error: "session" }, 400);
      }
      const lines = text.split("\n").filter((l) => l.trim()).length;
      if (lines < 2 && snaps < 1) {
        return json({ error: "empty" }, 400);
      }
      await env.PLAYS.put("f:" + sessionID, text, { expirationTtl: TTL });
      const saved = {
        session_id: sessionID,
        build: String(meta.build || "").slice(0, 40),
        snaps: snaps || Math.max(0, lines - 1),
        saved: Date.now(),
      };
      await env.PLAYS.put("m:" + sessionID, JSON.stringify(saved), { expirationTtl: TTL });
      const stats = await readStats(env);
      const had = await env.PLAYS.get("s:" + sessionID);
      if (!had) {
        await bumpSession(env, sessionID, saved.snaps, saved.build);
      }
      const after = await readStats(env);
      if (!JSON.parse((await env.PLAYS.get("m:" + sessionID + ":counted")) || "null")) {
        after.films = (after.films || 0) + 1;
        await writeStats(env, after);
        await env.PLAYS.put("m:" + sessionID + ":counted", "1", { expirationTtl: TTL });
      }
      return json({ ok: true, session_id: sessionID, snaps: saved.snaps, films: after.films || 1 });
    }

    if (req.method === "POST" && (path === "/" || path === "")) {
      let body;
      try {
        body = await req.json();
      } catch {
        return json({ error: "json" }, 400);
      }
      const sessionID = cleanSessionID(body.session_id);
      const snaps = Math.floor(Number(body.snaps));
      if (!sessionID || sessionID.length < 8 || !Number.isFinite(snaps) || snaps < 1 || snaps > MAX_SNAPS) {
        return json({ error: "fields" }, 400);
      }
      return json(await bumpSession(env, sessionID, snaps, body.build));
    }

    return json({ error: "not found" }, 404);
  },
};
