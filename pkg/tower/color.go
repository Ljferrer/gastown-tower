package tower

import "hash/fnv"

// TownColor is the reserved accent for town/HQ agents (amber).
const TownColor = "#e8a33d"

// rigPalette holds perceptually-spaced hues assigned to rigs by name hash, so a
// rig keeps a stable color across runs and clients (TUI + web).
var rigPalette = []string{
	"#3da5e8", // blue
	"#5ad17a", // green
	"#c77dff", // violet
	"#ff7d9c", // pink
	"#4fd6c8", // teal
	"#ff8c42", // orange
	"#9d8cff", // indigo
	"#e8d44d", // gold
}

// GroupColor returns a stable hex color for a group key. The town group gets the
// reserved amber accent; every other group (a rig) is assigned a palette hue by
// a deterministic hash of its name.
func GroupColor(group string) string {
	if group == townGroup {
		return TownColor
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(group))
	return rigPalette[h.Sum32()%uint32(len(rigPalette))]
}
