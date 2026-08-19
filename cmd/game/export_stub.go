//go:build !js

package main

func registerFilmExport(func() string) {}

func notifySnap(int, string, string) {}

// persistedSessionID: desktop has no cross-launch identity to reuse (and no
// need for one — it isn't sending anything anywhere). Fresh ID each run.
func persistedSessionID() string { return newSessionID() }
