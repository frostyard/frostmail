//go:build !race

package store

// raceEnabled is true under -race, which slows code several times over, so
// tests skip their time budgets but keep checking results.
const raceEnabled = false
