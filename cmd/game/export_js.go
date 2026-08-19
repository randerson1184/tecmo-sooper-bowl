//go:build js

package main

import "syscall/js"

func registerFilmExport(fn func() string) {
	js.Global().Set("tecmoExportFilm", js.FuncOf(func(this js.Value, args []js.Value) any {
		if fn == nil {
			return ""
		}
		return fn()
	}))
}

func notifySnap(n int, sessionID, build string) {
	fn := js.Global().Get("tecmoOnSnap")
	if fn.Type() != js.TypeFunction {
		return
	}
	fn.Invoke(n, sessionID, build)
}

// persistedSessionID reuses the same anonymous ID across reloads in this
// browser (localStorage), so the playcount measures returning people, not
// page loads. Falls back to a fresh one if storage is unavailable (private
// browsing, sandboxed frame, etc.) — never fatal.
func persistedSessionID() (id string) {
	defer func() {
		if recover() != nil {
			id = newSessionID()
		}
	}()
	const key = "tecmo_session_id"
	ls := js.Global().Get("localStorage")
	if ls.IsUndefined() || ls.IsNull() {
		return newSessionID()
	}
	if v := ls.Call("getItem", key); v.Type() == js.TypeString && v.String() != "" {
		return v.String()
	}
	id = newSessionID()
	ls.Call("setItem", key, id)
	return id
}
