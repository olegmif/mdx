package embed

import (
	"testing"

	"github.com/google/uuid"
)

func TestPointIDDeterministic(t *testing.T) {
	a := PointID("/notes/a.md")
	b := PointID("/notes/a.md")
	if a != b {
		t.Errorf("PointID not deterministic: %s vs %s", a, b)
	}
}

func TestPointIDDistinctPaths(t *testing.T) {
	a := PointID("/notes/a.md")
	b := PointID("/notes/b.md")
	if a == b {
		t.Errorf("PointID collision for distinct paths: both %s", a)
	}
}

func TestPointIDVersionAndVariant(t *testing.T) {
	id := PointID("/notes/a.md")
	if got, want := id.Version(), uuid.Version(5); got != want {
		t.Errorf("version = %d, want %d", got, want)
	}
	if got, want := id.Variant(), uuid.RFC4122; got != want {
		t.Errorf("variant = %v, want %v", got, want)
	}
}
