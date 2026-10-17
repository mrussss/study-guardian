package main

// A short grace permits reading between interactions. ActivityWatch remains
// an independent fact source; its unavailable state is never made active here.
func inputIsAFK(watcherAFK bool, idleSeconds int64, available bool) bool {
	return watcherAFK || available && idleSeconds >= 30
}
