//go:build race

package index

// slowdown is how much longer the race detector makes the timed test allow.
const slowdown = 10
