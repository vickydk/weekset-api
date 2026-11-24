// Package exercise is the fixed library of solo-day movements.
// A coached session log does not have to use these names.
package exercise

// Movement is one licensed or original loop.
type Movement struct {
	Slug string
	Name string
}

// Catalog is the twelve movements seeded into exercises.
var Catalog = []Movement{
	{Slug: "squat", Name: "squat"},
	{Slug: "hip-hinge", Name: "hip hinge"},
	{Slug: "lunge", Name: "lunge"},
	{Slug: "glute-bridge", Name: "glute bridge"},
	{Slug: "push-up", Name: "push-up"},
	{Slug: "dumbbell-bench-press", Name: "dumbbell bench press"},
	{Slug: "seated-row", Name: "seated row"},
	{Slug: "lat-pulldown", Name: "lat pulldown"},
	{Slug: "overhead-press", Name: "overhead press"},
	{Slug: "plank", Name: "plank"},
	{Slug: "dead-bug", Name: "dead bug"},
	{Slug: "farmer-carry", Name: "farmer carry"},
}
