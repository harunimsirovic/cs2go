package models

import "fmt"

// AdvancedStats holds context-aware performance metrics beyond raw kill counts.
type AdvancedStats struct {
	// Clutch Performance
	Clutch1v1Wins      int `json:"clutch_1v1_wins"`
	Clutch1v1Total     int `json:"clutch_1v1_total"`
	Clutch1v2Wins      int `json:"clutch_1v2_wins"`
	Clutch1v2Total     int `json:"clutch_1v2_total"`
	Clutch1v3Wins      int `json:"clutch_1v3_wins"`
	Clutch1v3Total     int `json:"clutch_1v3_total"`
	Clutch1v4PlusWins  int `json:"clutch_1v4plus_wins"` // 1v4 and 1v5 combined
	Clutch1v4PlusTotal int `json:"clutch_1v4plus_total"`

	// Entry Fragging (first kill/death each round)
	OpeningKills        int `json:"opening_kills"`         // got first kill of the round
	OpeningDeaths       int `json:"opening_deaths"`        // died first in the round
	OpeningKillAttempts int `json:"opening_kill_attempts"` // rounds where you went for opening (kill or death)

	// Trading
	TradeKills     int `json:"trade_kills"`     // got revenge kill within 5s of teammate death
	TradedDeaths   int `json:"traded_deaths"`   // teammate got revenge kill within 5s of your death
	UntradedDeaths int `json:"untraded_deaths"` // died and no one avenged you (baited)

	// Weapon Efficiency (kills per weapon buy)
	AKKills   int `json:"ak_kills"`
	AKRounds  int `json:"ak_rounds"` // rounds where you bought an AK
	M4Kills   int `json:"m4_kills"`
	M4Rounds  int `json:"m4_rounds"`
	AWPKills  int `json:"awp_kills"`
	AWPRounds int `json:"awp_rounds"`

	// Death Context
	DeathsWhileFlashed int `json:"deaths_while_flashed"`  // killed while blind
	DeathsFromBehind   int `json:"deaths_from_behind"`    // shot in back (bad positioning)
	DeathsWithUtilLeft int `json:"deaths_with_util_left"` // died holding unused nades

	// Multi-kill Rounds
	ThreeKillRounds int `json:"three_kill_rounds"` // 3K
	FourKillRounds  int `json:"four_kill_rounds"`  // 4K
	AceRounds       int `json:"ace_rounds"`        // 5K

	// Round Participation
	RoundsWithDamage     int `json:"rounds_with_damage"`      // dealt >0 damage
	RoundsWithZeroDamage int `json:"rounds_with_zero_damage"` // AFK or too passive

	// Impact Rating (weighted kills)
	ImpactScore float64 `json:"impact_score"` // HLTV-style weighted kill value
}

// GenerateAdvancedInsights produces insights from advanced stats.
func GenerateAdvancedInsights(basic *PlayerStats, adv *AdvancedStats) []Insight {
	var insights []Insight

	// Clutch performance
	total1vX := adv.Clutch1v1Total + adv.Clutch1v2Total + adv.Clutch1v3Total + adv.Clutch1v4PlusTotal
	if total1vX >= 3 {
		wins1vX := adv.Clutch1v1Wins + adv.Clutch1v2Wins + adv.Clutch1v3Wins + adv.Clutch1v4PlusWins
		winRate := float64(wins1vX) / float64(total1vX) * 100

		if winRate >= 40 {
			insights = append(insights, Insight{
				Category: "Clutch",
				Severity: "tip",
				Message:  fmt.Sprintf("Clutch win rate: %.0f%% (%d/%d). Excellent composure under pressure.", winRate, wins1vX, total1vX),
			})
		} else if winRate < 20 {
			insights = append(insights, Insight{
				Category: "Clutch",
				Severity: "warning",
				Message:  fmt.Sprintf("Clutch win rate: %.0f%% (%d/%d). Focus on isolating 1v1 duels and using utility.", winRate, wins1vX, total1vX),
			})
		}
	}

	// Entry fragging
	if adv.OpeningKillAttempts >= 5 {
		entrySuccessRate := float64(adv.OpeningKills) / float64(adv.OpeningKillAttempts) * 100
		if entrySuccessRate >= 50 {
			insights = append(insights, Insight{
				Category: "Entry",
				Severity: "tip",
				Message:  fmt.Sprintf("Opening duel success: %.0f%% — your aggression creates space.", entrySuccessRate),
			})
		} else if entrySuccessRate < 30 {
			insights = append(insights, Insight{
				Category: "Entry",
				Severity: "warning",
				Message:  fmt.Sprintf("Opening duel success: %.0f%% — entry with teammates for trades.", entrySuccessRate),
			})
		}
	}

	// Trading
	if basic.Deaths >= 5 {
		tradeRate := float64(adv.TradedDeaths) / float64(basic.Deaths) * 100
		baitRate := float64(adv.UntradedDeaths) / float64(basic.Deaths) * 100

		if baitRate >= 40 {
			insights = append(insights, Insight{
				Category: "Teamplay",
				Severity: "critical",
				Message:  fmt.Sprintf("You're baited %.0f%% of deaths — play closer with teammates.", baitRate),
			})
		} else if tradeRate >= 60 {
			insights = append(insights, Insight{
				Category: "Teamplay",
				Severity: "tip",
				Message:  fmt.Sprintf("Traded %.0f%% of deaths — excellent team positioning.", tradeRate),
			})
		}
	}

	// Weapon efficiency
	if adv.AWPRounds >= 3 {
		awpEff := float64(adv.AWPKills) / float64(adv.AWPRounds)
		if awpEff < 0.5 {
			insights = append(insights, Insight{
				Category: "Economy",
				Severity: "warning",
				Message:  fmt.Sprintf("AWP efficiency: %.1f kills/buy (avg: 0.7). Consider rifling.", awpEff),
			})
		} else if awpEff >= 1.0 {
			insights = append(insights, Insight{
				Category: "Economy",
				Severity: "tip",
				Message:  fmt.Sprintf("AWP efficiency: %.1f kills/buy — keep using the AWP.", awpEff),
			})
		}
	}

	// Death context
	if basic.Deaths >= 5 {
		flashedDeathRate := float64(adv.DeathsWhileFlashed) / float64(basic.Deaths) * 100
		if flashedDeathRate >= 35 {
			insights = append(insights, Insight{
				Category: "Game Sense",
				Severity: "warning",
				Message:  fmt.Sprintf("%.0f%% of deaths while flashed — turn away or wait out enemy utility.", flashedDeathRate),
			})
		}

		utilWastedRate := float64(adv.DeathsWithUtilLeft) / float64(basic.Deaths) * 100
		if utilWastedRate >= 50 {
			insights = append(insights, Insight{
				Category: "Utility",
				Severity: "critical",
				Message:  fmt.Sprintf("%.0f%% of deaths with nades unused — use utility before duels.", utilWastedRate),
			})
		}
	}

	// Multi-kills
	if adv.AceRounds >= 1 {
		insights = append(insights, Insight{
			Category: "Impact",
			Severity: "tip",
			Message:  fmt.Sprintf("%d ace(s) this game — massive round-winning performances.", adv.AceRounds),
		})
	}

	// Impact rating
	if adv.ImpactScore > 0 {
		avgImpact := adv.ImpactScore / float64(basic.Kills+1)
		if avgImpact >= 1.4 {
			insights = append(insights, Insight{
				Category: "Impact",
				Severity: "tip",
				Message:  fmt.Sprintf("Impact rating: %.1f/kill — your frags win rounds.", avgImpact),
			})
		}
	}

	return insights
}
