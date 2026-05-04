package embed

import "github.com/google/uuid"

// pointNamespace is the fixed UUID used as namespace for v5 point ids.
// Generated once with uuidgen and treated as a constant: changing it
// invalidates every previously written point in Qdrant, since the new
// id will differ from the existing one for the same note path.
var pointNamespace = uuid.MustParse("0c4f3804-15e0-4fde-b7e3-457688dc245d")

// PointID returns the deterministic UUID v5 for a note path. The same
// absolute path always maps to the same id — that is what makes the
// upsert in Qdrant idempotent.
func PointID(absPath string) uuid.UUID {
	return uuid.NewSHA1(pointNamespace, []byte(absPath))
}
