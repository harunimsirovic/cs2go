package models

// DemoHeader holds metadata from the demo file header.
type DemoHeader struct {
	MapName  string
	TickRate float64
}

// DemoResult holds all parsed data for a single .dem file.
type DemoResult struct {
	JobID    string                  `json:"job_id"`
	MapName  string                  `json:"map_name"`
	Duration float64                 `json:"duration_seconds"`
	Rounds   int                     `json:"total_rounds"`
	Players  map[uint64]*PlayerStats `json:"players"`
	RoundLog []RoundSnapshot         `json:"round_log"`
	KillLog  []string                `json:"kill_log"`
}

// PlayerStats aggregates all per-player metrics across the demo.
type PlayerStats struct {
	SteamID uint64 `json:"steam_id"`
	Name    string `json:"name"`
	Team    string `json:"team"`

	// Aim & Combat
	ShotsFired    int `json:"shots_fired"`
	ShotsHit      int `json:"shots_hit"`
	Kills         int `json:"kills"`
	Deaths        int `json:"deaths"`
	Assists       int `json:"assists"`
	HeadshotKills int `json:"headshot_kills"`

	// Hit location breakdown: "Head", "Chest", "Stomach", etc.
	HitLocations map[string]int `json:"hit_locations"`

	// Weapon usage: weapon name → shots fired
	WeaponUsage map[string]int `json:"weapon_usage"`

	// Utility
	GrenadesThrown int `json:"grenades_thrown"`
	FlashesThrown  int `json:"flashes_thrown"`
	SmokesThrown   int `json:"smokes_thrown"`
	MolotovsThrown int `json:"molotovs_thrown"`
	FlashAssists   int `json:"flash_assists"`

	// Economy
	TotalMoneySpent   int `json:"total_money_spent"`
	RoundsWithFullBuy int `json:"rounds_with_full_buy"`
	RoundsWithEco     int `json:"rounds_with_eco"`

	// Movement
	TotalDistanceMoved float64 `json:"total_distance_moved"`
	AverageSpeed       float64 `json:"average_speed"`
}

// RoundSnapshot captures the outcome of each round.
type RoundSnapshot struct {
	Number    int      `json:"round"`
	Winner    string   `json:"winner"`
	Reason    string   `json:"reason"`
	Duration  float64  `json:"duration_seconds"`
	TickDelta int      `json:"tick_delta,omitempty"`
	KillLog   []string `json:"kill_log"`
}

// Insight is a generated tip for a player based on their stats.
type Insight struct {
	Category string `json:"category"`
	Severity string `json:"severity"` // "tip", "warning", "critical"
	Message  string `json:"message"`
}

// GenerateInsights produces actionable feedback from a player's stats.
func GenerateInsights(s *PlayerStats) []Insight {
	var insights []Insight

	// Accuracy
	if s.ShotsFired > 0 {
		acc := float64(s.ShotsHit) / float64(s.ShotsFired) * 100
		if acc < 20 {
			insights = append(insights, Insight{
				Category: "Aim",
				Severity: "critical",
				Message:  "Accuracy below 20%. Focus on burst-firing instead of spraying — CS2 rewards patience over volume.",
			})
		} else if acc < 35 {
			insights = append(insights, Insight{
				Category: "Aim",
				Severity: "warning",
				Message:  "Moderate accuracy. Try counter-strafing (tap A/D) before shooting to reduce spread.",
			})
		}
	}

	// Headshot ratio
	if s.Kills > 0 {
		hsRate := float64(s.HeadshotKills) / float64(s.Kills) * 100
		if hsRate < 20 {
			insights = append(insights, Insight{
				Category: "Aim",
				Severity: "tip",
				Message:  "Low headshot rate. Lower your crosshair placement to natural head-height on common angles.",
			})
		}
	}

	// Utility usage
	totalUtil := s.GrenadesThrown + s.FlashesThrown + s.SmokesThrown + s.MolotovsThrown
	if totalUtil < 3 {
		insights = append(insights, Insight{
			Category: "Utility",
			Severity: "warning",
			Message:  "Very low utility usage. Smokes and flashes win rounds — buy and use them every round.",
		})
	}

	// Economy
	if s.RoundsWithEco > 5 {
		insights = append(insights, Insight{
			Category: "Economy",
			Severity: "tip",
			Message:  "Frequent eco rounds. Coordinate with teammates to force-buy together rather than splitting economy.",
		})
	}

	return insights
}
