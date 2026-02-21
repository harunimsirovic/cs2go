package parser

import (
	"fmt"
	"math"
	"time"

	"cs2-demo-analyzer/internal/models"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

// RoundContext tracks state needed for advanced stats (clutch, entry, trading).
type RoundContext struct {
	// Entry fragging
	firstKillTick  int
	firstKillSID   uint64
	firstDeathTick int
	firstDeathSID  uint64

	// Trading (death timestamp → victim SID)
	recentDeaths map[uint64]int // SID → tick of death

	// Clutch detection (alive count at round end)
	roundNumber int

	// Multi-kill tracking
	killsThisRound map[uint64]int

	// Weapon buys this round
	weaponBuys map[uint64]string // SID → primary weapon

	// Damage tracking
	damageDealt map[uint64]int
}

func registerExtractors(
	p demoinfocs.Parser,
	playerStats map[uint64]*models.PlayerStats,
	roundLog *[]models.RoundSnapshot,
	roundKillLogs map[int][]string,
	roundStartTick *int,
	currentRound *int,
) {
	ctx := &RoundContext{
		recentDeaths:   make(map[uint64]int),
		killsThisRound: make(map[uint64]int),
		weaponBuys:     make(map[uint64]string),
		damageDealt:    make(map[uint64]int),
	}

	// ── Round lifecycle ─────────────────────────────────────────────────────
	p.RegisterEventHandler(func(e events.RoundStart) {
		*currentRound++
		*roundStartTick = p.GameState().IngameTick()
		ctx.roundNumber = *currentRound

		// Reset round-specific trackers
		ctx.firstKillTick = 0
		ctx.firstKillSID = 0
		ctx.firstDeathTick = 0
		ctx.firstDeathSID = 0
		ctx.recentDeaths = make(map[uint64]int)
		ctx.killsThisRound = make(map[uint64]int)
		ctx.weaponBuys = make(map[uint64]string)
		ctx.damageDealt = make(map[uint64]int)
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
		// Clutch detection: count alive players per team at round end
		gs := p.GameState()
		tAlive, ctAlive := 0, 0
		for _, player := range gs.Participants().Playing() {
			if player.IsAlive() {
				if player.Team == common.TeamTerrorists {
					tAlive++
				} else if player.Team == common.TeamCounterTerrorists {
					ctAlive++
				}
			}
		}

		// If round ended with 1 alive vs N dead, that's a clutch win
		for _, player := range gs.Participants().Playing() {
			if !player.IsAlive() {
				continue
			}
			s := getOrCreate(playerStats, player)

			var enemies int
			if player.Team == common.TeamTerrorists {
				enemies = ctAlive
			} else {
				enemies = tAlive
			}

			// This player is the last one alive and won
			var myTeamAlive int
			if player.Team == common.TeamTerrorists {
				myTeamAlive = tAlive
			} else {
				myTeamAlive = ctAlive
			}

			if myTeamAlive == 1 {
				// Determine how many enemies were alive when this player clutched
				// We infer from kills this round vs team sizes
				// Simpler: check winner and count initial team size - kills by this player
				// For now, simplified: check if 1vX at end
				switch enemies {
				case 1:
					s.Advanced.Clutch1v1Total++
					if (player.Team == common.TeamTerrorists && e.Winner == common.TeamTerrorists) ||
						(player.Team == common.TeamCounterTerrorists && e.Winner == common.TeamCounterTerrorists) {
						s.Advanced.Clutch1v1Wins++
					}
				case 2:
					s.Advanced.Clutch1v2Total++
					if (player.Team == common.TeamTerrorists && e.Winner == common.TeamTerrorists) ||
						(player.Team == common.TeamCounterTerrorists && e.Winner == common.TeamCounterTerrorists) {
						s.Advanced.Clutch1v2Wins++
					}
				case 3:
					s.Advanced.Clutch1v3Total++
					if (player.Team == common.TeamTerrorists && e.Winner == common.TeamTerrorists) ||
						(player.Team == common.TeamCounterTerrorists && e.Winner == common.TeamCounterTerrorists) {
						s.Advanced.Clutch1v3Wins++
					}
				default:
					if enemies >= 4 {
						s.Advanced.Clutch1v4PlusTotal++
						if (player.Team == common.TeamTerrorists && e.Winner == common.TeamTerrorists) ||
							(player.Team == common.TeamCounterTerrorists && e.Winner == common.TeamCounterTerrorists) {
							s.Advanced.Clutch1v4PlusWins++
						}
					}
				}
			}
		}

		// Multi-kill detection
		for sid, kills := range ctx.killsThisRound {
			if kills >= 5 {
				if s, ok := playerStats[sid]; ok {
					s.Advanced.AceRounds++
				}
			} else if kills == 4 {
				if s, ok := playerStats[sid]; ok {
					s.Advanced.FourKillRounds++
				}
			} else if kills == 3 {
				if s, ok := playerStats[sid]; ok {
					s.Advanced.ThreeKillRounds++
				}
			}
		}

		// Round participation (damage tracking)
		for sid, dmg := range ctx.damageDealt {
			if s, ok := playerStats[sid]; ok {
				if dmg > 0 {
					s.Advanced.RoundsWithDamage++
				} else {
					s.Advanced.RoundsWithZeroDamage++
				}
			}
		}

		// Store round snapshot
		tickDelta := p.GameState().IngameTick() - *roundStartTick
		if tickDelta < 0 {
			tickDelta = 0
		}
		*roundLog = append(*roundLog, models.RoundSnapshot{
			Number:    *currentRound,
			Winner:    teamName(e.Winner),
			Reason:    reasonName(e.Reason),
			TickDelta: tickDelta,
			KillLog:   getRoundKillLog(roundKillLogs, *currentRound),
		})
	})

	// ── Weapon buys (for efficiency tracking) ───────────────────────────────
	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		for _, player := range p.GameState().Participants().Playing() {
			s := getOrCreate(playerStats, player)

			// Detect primary weapon
			for _, weapon := range player.Weapons() {
				wType := weapon.Type
				wName := weapon.String()

				// Track AK/M4/AWP rounds
				if wType == common.EqAK47 {
					s.Advanced.AKRounds++
					ctx.weaponBuys[player.SteamID64] = "AK-47"
				} else if wType == common.EqM4A4 || wType == common.EqM4A1 {
					s.Advanced.M4Rounds++
					ctx.weaponBuys[player.SteamID64] = wName
				} else if wType == common.EqAWP {
					s.Advanced.AWPRounds++
					ctx.weaponBuys[player.SteamID64] = "AWP"
				}
			}

			// Eco/full buy classification
			equip := player.EquipmentValueCurrent()
			switch {
			case equip >= 4000:
				s.RoundsWithFullBuy++
			case equip <= 1000:
				s.RoundsWithEco++
			}
		}
	})

	// ── Combat: shots fired ─────────────────────────────────────────────────
	p.RegisterEventHandler(func(e events.WeaponFire) {
		if e.Shooter == nil {
			return
		}
		s := getOrCreate(playerStats, e.Shooter)
		s.ShotsFired++
		s.WeaponUsage[e.Weapon.String()]++
	})

	// ── Combat: shots hit + damage ──────────────────────────────────────────
	p.RegisterEventHandler(func(e events.PlayerHurt) {
		if e.Attacker == nil || e.Attacker == e.Player {
			return
		}
		s := getOrCreate(playerStats, e.Attacker)
		s.ShotsHit++
		s.HitLocations[hitGroupName(e.HitGroup)]++

		// Track damage for round participation
		ctx.damageDealt[e.Attacker.SteamID64] += e.HealthDamage
	})

	// ── Combat: kills / deaths / entry / trading ────────────────────────────
	p.RegisterEventHandler(func(e events.Kill) {
		tick := p.GameState().IngameTick()
		var hs string
		var killOption = " "

		if e.Killer != nil && e.Killer != e.Victim {
			s := getOrCreate(playerStats, e.Killer)
			s.Kills++

			// Multi-kill tracking
			ctx.killsThisRound[e.Killer.SteamID64]++

			// Weapon efficiency
			weapon := e.Weapon.String()
			if weapon == "AK-47" {
				s.Advanced.AKKills++
			} else if weapon == "M4A4" || weapon == "M4A1" {
				s.Advanced.M4Kills++
			} else if weapon == "AWP" {
				s.Advanced.AWPKills++
			}

			// Entry fragging (first kill of the round)
			if ctx.firstKillTick == 0 {
				ctx.firstKillTick = tick
				ctx.firstKillSID = e.Killer.SteamID64
				s.Advanced.OpeningKills++
				s.Advanced.OpeningKillAttempts++

				// Impact: opening kill = 2.0x
				s.Advanced.ImpactScore += 2.0
			} else {
				// Regular kill = 1.0x
				s.Advanced.ImpactScore += 1.0
			}

			// Trading detection: did this kill happen within 5s of a teammate death?
			for sid, deathTick := range ctx.recentDeaths {
				// Same team as dead player, different SID
				if deadPlayer := findPlayer(p, sid); deadPlayer != nil {
					if deadPlayer.Team == e.Killer.Team && sid != e.Killer.SteamID64 {
						tickDelta := tick - deathTick
						// 5 seconds = 5 * 128 ticks (assuming 128 tick server)
						if tickDelta <= 640 { // ~5s
							s.Advanced.TradeKills++
							// Trade kill = 1.2x
							s.Advanced.ImpactScore += 0.2

							// Mark the dead player as traded
							if deadStats, ok := playerStats[sid]; ok {
								deadStats.Advanced.TradedDeaths++
							}
							break
						}
					}
				}
			}

			// Clutch kill weighting
			aliveTeammates := countAlive(p, e.Killer.Team)
			if aliveTeammates == 1 {
				// In clutch, weight by enemies alive
				enemiesAlive := countAlive(p, oppositeTeam(e.Killer.Team))
				s.Advanced.ImpactScore += float64(enemiesAlive) * 0.3
			}
		}

		if e.Victim != nil {
			s := getOrCreate(playerStats, e.Victim)
			s.Deaths++

			// Entry fragging (first death of the round)
			if ctx.firstDeathTick == 0 {
				ctx.firstDeathTick = tick
				ctx.firstDeathSID = e.Victim.SteamID64
				s.Advanced.OpeningDeaths++

				// If you die first AND got opening kill, count as entry attempt
				if ctx.firstKillSID != e.Victim.SteamID64 {
					s.Advanced.OpeningKillAttempts++
				}
			}

			// Death context: flashed?
			if e.Victim.IsBlinded() {
				s.Advanced.DeathsWhileFlashed++
			}

			// Death context: from behind?
			// Heuristic: if victim was shot in the back (simplified, needs view angles for true calc)
			// For now, skip this — requires player.ViewDirectionX/Y vs attacker position delta

			// Death context: utility left?
			utilLeft := 0
			for _, weap := range e.Victim.Weapons() {
				if weap.Type == common.EqFlash || weap.Type == common.EqSmoke ||
					weap.Type == common.EqMolotov || weap.Type == common.EqIncendiary ||
					weap.Type == common.EqHE {
					utilLeft++
				}
			}
			if utilLeft > 0 {
				s.Advanced.DeathsWithUtilLeft++
			}

			// Track recent deaths for trading
			ctx.recentDeaths[e.Victim.SteamID64] = tick
		}

		if e.Assister != nil {
			s := getOrCreate(playerStats, e.Assister)
			s.Assists++
			if e.AssistedFlash {
				s.FlashAssists++
			}
		}

		if e.PenetratedObjects > 0 {
			killOption = killOption + " /🧱/"
		}

		if e.ThroughSmoke == true {
			killOption = killOption + " (😶‍🌫️)"
		}

		killLog := fmt.Sprintf("%s (%v%s%s) %s\n",
			e.Killer,
			e.Weapon,
			hs,
			killOption,
			e.Victim,
		)
		appendRoundKillLog(roundKillLogs, *currentRound, killLog)

	})

	// ── Utility: grenades ───────────────────────────────────────────────────
	p.RegisterEventHandler(func(e events.GrenadeProjectileThrow) {
		thrower := e.Projectile.Thrower
		if thrower == nil {
			return
		}
		s := getOrCreate(playerStats, thrower)
		s.GrenadesThrown++

		switch e.Projectile.WeaponInstance.Type {
		case common.EqFlash:
			s.FlashesThrown++
		case common.EqSmoke:
			s.SmokesThrown++
		case common.EqMolotov, common.EqIncendiary:
			s.MolotovsThrown++
		}
	})

	// ── Economy: money spent ────────────────────────────────────────────────
	p.RegisterEventHandler(func(e events.RoundEnd) {
		for _, player := range p.GameState().Participants().Playing() {
			getOrCreate(playerStats, player).TotalMoneySpent += player.EquipmentValueCurrent()
		}
	})

	// ── Movement ────────────────────────────────────────────────────────────
	lastPos := make(map[uint64][3]float64)

	p.RegisterEventHandler(func(e events.FrameDone) {
		for _, player := range p.GameState().Participants().Playing() {
			if !player.IsAlive() {
				continue
			}
			sid := player.SteamID64
			pos := player.Position()
			cur := [3]float64{pos.X, pos.Y, pos.Z}

			if prev, ok := lastPos[sid]; ok {
				dx := cur[0] - prev[0]
				dy := cur[1] - prev[1]
				dz := cur[2] - prev[2]
				getOrCreate(playerStats, player).TotalDistanceMoved += math.Sqrt(dx*dx + dy*dy + dz*dz)
			}
			lastPos[sid] = cur
		}
	})

}

// ── Helpers ──────────────────────────────────────────────────────────────────

func getOrCreate(m map[uint64]*models.PlayerStats, player *common.Player) *models.PlayerStats {
	sid := player.SteamID64
	if s, ok := m[sid]; ok {
		return s
	}
	s := &models.PlayerStats{
		SteamID:      sid,
		Name:         player.Name,
		Team:         teamName(player.Team),
		HitLocations: make(map[string]int),
		WeaponUsage:  make(map[string]int),
	}
	m[sid] = s
	return s
}

func findPlayer(p demoinfocs.Parser, sid uint64) *common.Player {
	for _, player := range p.GameState().Participants().All() {
		if player.SteamID64 == sid {
			return player
		}
	}
	return nil
}

func countAlive(p demoinfocs.Parser, team common.Team) int {
	count := 0
	for _, player := range p.GameState().Participants().Playing() {
		if player.Team == team && player.IsAlive() {
			count++
		}
	}
	return count
}

func oppositeTeam(t common.Team) common.Team {
	if t == common.TeamTerrorists {
		return common.TeamCounterTerrorists
	}
	return common.TeamTerrorists
}

func teamName(t common.Team) string {
	switch t {
	case common.TeamTerrorists:
		return "T"
	case common.TeamCounterTerrorists:
		return "CT"
	case common.TeamSpectators:
		return "Spectator"
	default:
		return "Unassigned"
	}
}

func reasonName(r events.RoundEndReason) string {
	switch r {
	case events.RoundEndReasonTargetBombed:
		return "BombExploded"
	case events.RoundEndReasonBombDefused:
		return "BombDefused"
	case events.RoundEndReasonCTWin:
		return "CTElimination"
	case events.RoundEndReasonTerroristsWin:
		return "TElimination"
	case events.RoundEndReasonTargetSaved:
		return "TimerExpired"
	case events.RoundEndReasonHostagesRescued:
		return "HostagesRescued"
	default:
		return fmt.Sprintf("Reason(%d)", int(r))
	}
}

func hitGroupName(hg events.HitGroup) string {
	switch hg {
	case events.HitGroupHead:
		return "Head"
	case events.HitGroupChest:
		return "Chest"
	case events.HitGroupStomach:
		return "Stomach"
	case events.HitGroupLeftArm:
		return "LeftArm"
	case events.HitGroupRightArm:
		return "RightArm"
	case events.HitGroupLeftLeg:
		return "LeftLeg"
	case events.HitGroupRightLeg:
		return "RightLeg"
	case events.HitGroupNeck:
		return "Neck"
	default:
		return "Unknown"
	}
}

func appendRoundKillLog(roundKillLogs map[int][]string, round int, killLog string) {
	if round <= 0 {
		return
	}
	roundKillLogs[round] = append(roundKillLogs[round], killLog)
}

func getRoundKillLog(roundKillLogs map[int][]string, round int) []string {
	if round <= 0 {
		return nil
	}
	logs := roundKillLogs[round]
	if len(logs) == 0 {
		return nil
	}
	return append([]string(nil), logs...)
}

// unused import guard
var _ = time.Second
