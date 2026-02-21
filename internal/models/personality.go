package models

// PlayerPersonality classifies a player's playstyle based on their stats.
type PlayerPersonality struct {
	PrimaryRole   string   `json:"primary_role"`   // "Entry Fragger", "Support Anchor", etc.
	SecondaryRole string   `json:"secondary_role"` // Optional secondary classification
	Strengths     []string `json:"strengths"`      // What they do well
	Suggestions   []string `json:"suggestions"`    // How to improve/adapt
	Confidence    float64  `json:"confidence"`     // 0-1, how confident is the classification
}

// ClassifyPlayer determines a player's personality/role from their stats.
func ClassifyPlayer(basic *PlayerStats, adv *AdvancedStats) PlayerPersonality {
	// Minimum sample size for confident classification
	if basic.Kills < 5 {
		return PlayerPersonality{
			PrimaryRole: "Insufficient Data",
			Confidence:  0.0,
		}
	}

	scores := make(map[string]float64)
	strengths := make(map[string][]string)
	suggestions := make(map[string][]string)

	// ── Entry Fragger ────────────────────────────────────────────────────────
	// High opening kills, high first deaths, aggressive positioning
	if adv.OpeningKillAttempts > 0 {
		entryRate := float64(adv.OpeningKills) / float64(adv.OpeningKillAttempts)
		if entryRate > 0.4 {
			scores["Entry Fragger"] += 3.0
			strengths["Entry Fragger"] = append(strengths["Entry Fragger"],
				"Creates space with aggressive entries")
		}
		if adv.OpeningDeaths > adv.OpeningKills {
			scores["Entry Fragger"] += 1.0 // Dies a lot trying = entry style
		}
	}

	// ── Support Anchor ───────────────────────────────────────────────────────
	// High flash assists, trades teammates, lower opening kills
	if basic.FlashAssists > 2 {
		scores["Support Anchor"] += 2.0
		strengths["Support Anchor"] = append(strengths["Support Anchor"],
			"Excellent utility usage for teammates")
	}
	if adv.TradeKills > 3 {
		scores["Support Anchor"] += 2.0
		strengths["Support Anchor"] = append(strengths["Support Anchor"],
			"Reliable at trading fallen teammates")
	}
	if adv.OpeningKills < 2 && basic.Kills > 5 {
		scores["Support Anchor"] += 1.0 // Gets kills, but not opening = support
	}

	// ── Clutch King ──────────────────────────────────────────────────────────
	// High 1vX win rate, calm under pressure
	total1vX := adv.Clutch1v1Total + adv.Clutch1v2Total + adv.Clutch1v3Total + adv.Clutch1v4PlusTotal
	if total1vX >= 3 {
		wins1vX := adv.Clutch1v1Wins + adv.Clutch1v2Wins + adv.Clutch1v3Wins + adv.Clutch1v4PlusWins
		clutchRate := float64(wins1vX) / float64(total1vX)
		if clutchRate > 0.35 {
			scores["Clutch King"] += 3.0
			strengths["Clutch King"] = append(strengths["Clutch King"],
				"Excels in high-pressure 1vX situations")
		}
	}

	// ── AWP Specialist ───────────────────────────────────────────────────────
	// High AWP usage, good AWP efficiency
	if adv.AWPRounds > 3 {
		awpEff := float64(adv.AWPKills) / float64(adv.AWPRounds)
		if awpEff > 0.7 {
			scores["AWP Specialist"] += 3.0
			strengths["AWP Specialist"] = append(strengths["AWP Specialist"],
				"Strong AWP fundamentals and positioning")
		}
		// Heavy AWP usage regardless of efficiency
		awpUsageRate := float64(adv.AWPRounds) / float64(len(basic.WeaponUsage))
		if awpUsageRate > 0.3 {
			scores["AWP Specialist"] += 1.5
		}
	}

	// ── Lurker ───────────────────────────────────────────────────────────────
	// Low opening duels, but gets kills late in rounds, possibly from behind
	// (Placeholder — needs positional data for full detection)
	if adv.OpeningKillAttempts < 3 && basic.Kills > 8 {
		scores["Lurker"] += 1.5
		strengths["Lurker"] = append(strengths["Lurker"],
			"Picks off rotations and isolated targets")
	}

	// ── Rifler (All-Arounder) ────────────────────────────────────────────────
	// Good with AK/M4, balanced stats, no extreme specialization
	totalRifleRounds := adv.AKRounds + adv.M4Rounds
	if totalRifleRounds > 5 {
		rifleKills := adv.AKKills + adv.M4Kills
		rifleEff := float64(rifleKills) / float64(totalRifleRounds)
		if rifleEff > 0.8 {
			scores["Rifler"] += 2.0
			strengths["Rifler"] = append(strengths["Rifler"],
				"Consistent rifle performance")
		}
	}

	// ── Utility Bot ──────────────────────────────────────────────────────────
	// High nade usage, lower kills — pure support player
	totalUtil := basic.GrenadesThrown + basic.FlashesThrown + basic.SmokesThrown + basic.MolotovsThrown
	if totalUtil > 10 && basic.Kills < 8 {
		scores["Utility Bot"] += 2.0
		strengths["Utility Bot"] = append(strengths["Utility Bot"],
			"Sets up teammates with excellent utility")
	}

	// ── Eco Warrior ──────────────────────────────────────────────────────────
	// High force-buy participation, pistol round impact
	// (Needs pistol round tracking — placeholder for now)
	if basic.RoundsWithEco > 5 {
		scores["Eco Warrior"] += 1.0
	}

	// ── Find top role ────────────────────────────────────────────────────────
	var primary, secondary string
	var primaryScore, secondaryScore float64

	for role, score := range scores {
		if score > primaryScore {
			secondary = primary
			secondaryScore = primaryScore
			primary = role
			primaryScore = score
		} else if score > secondaryScore {
			secondary = role
			secondaryScore = score
		}
	}

	if primary == "" {
		primary = "Balanced Player"
		strengths[primary] = []string{"Well-rounded across all areas"}
		suggestions[primary] = []string{
			"Consider specializing in one role (entry, support, AWP) to maximize impact",
		}
	}

	// Calculate confidence (0-1) based on score separation
	confidence := 0.5
	if primaryScore > 0 {
		if secondaryScore == 0 {
			confidence = 0.9 // Clear winner
		} else {
			confidence = 0.5 + (primaryScore-secondaryScore)/primaryScore*0.4
		}
	}

	// Add role-specific suggestions
	switch primary {
	case "Entry Fragger":
		suggestions[primary] = []string{
			"Entry with a teammate for instant trades",
			"Use your utility before peeking (flash yourself in)",
			"Don't over-peek — get the opening and fall back",
		}
	case "Support Anchor":
		suggestions[primary] = []string{
			"Occasionally entry to keep enemies guessing",
			"Play post-plant to maximize your patient style",
			"Call out info for your entry fraggers",
		}
	case "Clutch King":
		suggestions[primary] = []string{
			"Don't force clutches — play with your team when possible",
			"Save utility for late-round scenarios",
			"Your composure is your strength — stay calm",
		}
	case "AWP Specialist":
		suggestions[primary] = []string{
			"Practice aggressive AWP peeks for opening picks",
			"Have a rifle backup plan if AWP isn't working",
			"Watch pro AWPers' positioning on your worst maps",
		}
	case "Rifler":
		suggestions[primary] = []string{
			"You're versatile — consider IGLing or supporting less flexible players",
			"Master spray control to push your rifle efficiency higher",
		}
	case "Utility Bot":
		suggestions[primary] = []string{
			"Balance utility usage with taking duels",
			"Learn aggressive nade lineups to create space",
			"Your utility wins rounds — keep it up",
		}
	}

	return PlayerPersonality{
		PrimaryRole:   primary,
		SecondaryRole: secondary,
		Strengths:     strengths[primary],
		Suggestions:   suggestions[primary],
		Confidence:    confidence,
	}
}
