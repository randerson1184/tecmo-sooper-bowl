// Anonymous play counter. Accepts {session_id, build, snaps} only.
// GET / → how many sessions actually snapped.

const MAX_SNAPS = 10000;

function corsHeaders() {
  return {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Methods": "GET, POST, OPTIONS",
    "Access-Control-Allow-Headers": "Content-Type",
  };
}

function json(data, status) {
  return new Response(JSON.stringify(data), {
    status: status || 200,
    headers: { "Content-Type": "application/json", ...corsHeaders() },
  });
}

export default {
  async fetch(req, env) {
    if (req.method === "OPTIONS") {
      return new Response(null, { status: 204, headers: corsHeaders() });
    }
    if (req.method === "GET") {
      const stats = JSON.parse((await env.PLAYS.get("stats")) || '{"sessions":0,"snaps":0}');
      return json({
        sessions_that_snapped: stats.sessions || 0,
        snaps_recorded: stats.snaps || 0,
        note: "A session counts when the first snap is recorded — not a page view.",
      });
    }
    if (req.method !== "POST") {
      return json({ error: "method" }, 405);
    }
    let body;
    try {
      body = await req.json();
    } catch {
      return json({ error: "json" }, 400);
    }
    const sessionID = String(body.session_id || "").replace(/[^a-fA-F0-9-]/g, "").slice(0, 32);
    const snaps = Math.floor(Number(body.snaps));
    if (!sessionID || sessionID.length < 8 || !Number.isFinite(snaps) || snaps < 1 || snaps > MAX_SNAPS) {
      return json({ error: "fields" }, 400);
    }
    const build = String(body.build || "").replace(/[^a-zA-Z0-9._-]/g, "").slice(0, 40);
    const key = "s:" + sessionID;
    const prev = JSON.parse((await env.PLAYS.get(key)) || "null");
    const now = Date.now();
    if (prev && now - (prev.last || 0) < 1500) {
      return json({ ok: true, throttled: true });
    }
    const rec = {
      session_id: sessionID,
      build,
      snaps: Math.max(snaps, prev ? prev.snaps : 0),
      first: prev ? prev.first : now,
      last: now,
    };
    await env.PLAYS.put(key, JSON.stringify(rec), { expirationTtl: 60 * 60 * 24 * 90 });
    const stats = JSON.parse((await env.PLAYS.get("stats")) || '{"sessions":0,"snaps":0}');
    if (!prev) {
      stats.sessions = (stats.sessions || 0) + 1;
    }
    stats.snaps = (stats.snaps || 0) + Math.max(0, rec.snaps - (prev ? prev.snaps : 0));
    await env.PLAYS.put("stats", JSON.stringify(stats));
    return json({ ok: true, sessions: stats.sessions, snaps: stats.snaps });
  },
};
