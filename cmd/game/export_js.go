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
