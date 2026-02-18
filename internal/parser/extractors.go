package parser

import (
	"fmt"
	"log"
	"math"

	"cs2-demo-analyzer/internal/models"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	common "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
)

// registerExtractors attaches all event handlers to the parser.
//
// v5 API notes (same as v4 for these specifics):
//   - common.Player.Name and .SteamID64 are FIELDS, not methods.
//   - GrenadeProjectileThrow: thrower is e.Projectile.Thrower.
//   - common.Team is type int — no .String(); use a switch.
//   - events.RoundEndReason is type int — no .String(); use a switch.
//   - events.FrameDone replaces the deprecated events.TickDone.
func registerExtractors(
	p demoinfocs.Parser,
	playerStats map[uint64]*models.PlayerStats,
	roundLog *[]models.RoundSnapshot,
	roundKillLogs map[int][]string,
	roundStartTick *int,
	currentRound *int,
) {
	// ── Combat: shots fired ─────────────────────────────────────────────────
	p.RegisterEventHandler(func(e events.WeaponFire) {
		if e.Shooter == nil {
			return
		}
		s := getOrCreate(playerStats, e.Shooter)
		s.ShotsFired++
		s.WeaponUsage[e.Weapon.String()]++
	})

	// ── Combat: shots hit + hit location breakdown ──────────────────────────
	p.RegisterEventHandler(func(e events.PlayerHurt) {
		if e.Attacker == nil || e.Attacker == e.Player {
			return
		}
		s := getOrCreate(playerStats, e.Attacker)
		s.ShotsHit++
		s.HitLocations[hitGroupName(e.HitGroup)]++
	})

	// ── Combat: kills / deaths / assists / headshots / flash-assists ─────────
	p.RegisterEventHandler(func(e events.Kill) {
		var hs string
		var killOption = " "

		if e.Killer != nil && e.Killer != e.Victim {
			s := getOrCreate(playerStats, e.Killer)
			s.Kills++
			if e.IsHeadshot {
				s.HeadshotKills++
				killOption = " 💥🪖"
			}
		}
		if e.Victim != nil {
			getOrCreate(playerStats, e.Victim).Deaths++
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

		killLog := fmt.Sprintf("%s =%v%s%s= %s\n",
			e.Killer,
			e.Weapon,
			hs,
			killOption,
			e.Victim,
		)
		appendRoundKillLog(roundKillLogs, *currentRound, killLog)
		log.Printf("%s =%v%s%s>= %s", e.Killer, e.Weapon, hs, killOption, e.Victim)

	})

	// ── Utility: grenade throws ─────────────────────────────────────────────
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

	// ── Economy: classify buy type at freeze-time end ────────────────────────
	p.RegisterEventHandler(func(e events.RoundFreezetimeEnd) {
		for _, player := range p.GameState().Participants().Playing() {
			s := getOrCreate(playerStats, player)
			equip := player.EquipmentValueCurrent()
			switch {
			case equip >= 4000:
				s.RoundsWithFullBuy++
			case equip <= 1000:
				s.RoundsWithEco++
			}
		}
	})

	// ── Economy: accumulate money spent per round ────────────────────────────
	p.RegisterEventHandler(func(e events.RoundEnd) {
		for _, player := range p.GameState().Participants().Playing() {
			getOrCreate(playerStats, player).TotalMoneySpent += player.EquipmentValueCurrent()
		}
	})

	// ── Movement: accumulate position delta each frame ──────────────────────
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

	// ── Round log ───────────────────────────────────────────────────────────
	p.RegisterEventHandler(func(e events.RoundStart) {
		*currentRound++
		*roundStartTick = p.GameState().IngameTick()
	})

	p.RegisterEventHandler(func(e events.RoundEnd) {
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
	case events.HitGroupGear:
		return "Gear"
	case events.HitGroupGeneric:
		return "Generic"
	default:
		return "Unknown"
	}
}
