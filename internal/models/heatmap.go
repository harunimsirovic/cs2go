package models

// Position represents a 3D coordinate in the game world.
type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// HeatmapData holds positional data for visualization.
type HeatmapData struct {
	Kills  []Position `json:"kills"`  // Where this player got kills
	Deaths []Position `json:"deaths"` // Where this player died
}

// HeatmapExport contains all heatmap data for a match, keyed by player SteamID.
type HeatmapExport struct {
	MapName string                 `json:"map_name"`
	Players map[uint64]HeatmapData `json:"players"`
}
