package parser

import (
	"fmt"
	"log"
	"math"

	"cs2-demo-analyzer/internal/models"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v5/pkg/demoinfocs/msg"
)

// ProgressFn is called periodically with a value 0–100.
type ProgressFn func(progress int)

// Parse opens a .dem file, runs the full event pipeline, and returns a DemoResult.
//
// v5 uses demoinfocs.ParseFile which handles file I/O, CS2 quirks, and cleanup
// internally — no NewParserWithConfig, no buffer tuning, no manual Close needed.
//
// Important: parsers are NOT goroutine-safe. Each call to Parse must run in its
// own goroutine — the worker pool in pipeline.go guarantees this.
func Parse(path string, onProgress ProgressFn) (*models.DemoResult, error) {
	playerStats := make(map[uint64]*models.PlayerStats)
	var roundLog []models.RoundSnapshot
	roundKillLogs := make(map[int][]string)
	roundStartTick := 0
	currentRound := 0
	var header models.DemoHeader

	err := demoinfocs.ParseFile(path, func(p demoinfocs.Parser) error {
		//// Capture header metadata.
		p.RegisterNetMessageHandler(func(m *msg.CSVCMsg_ServerInfo) {
			fmt.Println("Map:", m.GetMapName())
			log.Println(m.GetMapName())
			header = models.DemoHeader{
				MapName:  m.GetMapName(),
				TickRate: float64(m.GetTickInterval()),
			}
		})

		// Register all stat extractors.
		registerExtractors(p, playerStats, &roundLog, roundKillLogs, &roundStartTick, &currentRound)

		// Report progress on every RoundEnd.
		p.RegisterEventHandler(func(e events.RoundEnd) {
			if onProgress != nil {
				onProgress(int(p.Progress() * 100))
			}
		})

		p.RegisterNetMessageHandler(func(m *msg.CSVCMsg_ServerInfo) {
			fmt.Println("Map:", m.GetMapName())
			log.Println(m.GetMapName())
		})

		return nil
	})

	// v5 swallows CS2-specific panics and truncated-demo errors internally.
	// A non-nil error here is a genuine unrecoverable failure.
	if err != nil {
		return nil, fmt.Errorf("parsing demo: %w", err)
	}

	// Post-process: convert ticks → seconds and compute derived stats.
	tickRate := header.TickRate
	for _, s := range playerStats {
		computeDerivedStats(s, tickRate)
	}
	for i := range roundLog {
		if tickRate > 0 {
			roundLog[i].Duration = float64(roundLog[i].TickDelta) / tickRate
		}
	}

	return &models.DemoResult{
		MapName: header.MapName,
		Duration: func() float64 {
			if len(roundLog) == 0 || tickRate == 0 {
				return 0
			}
			total := 0
			for _, r := range roundLog {
				total += r.TickDelta
			}
			return float64(total) / tickRate
		}(),
		Rounds:   len(roundLog),
		Players:  playerStats,
		RoundLog: roundLog,
		KillLog:  flattenKillLog(roundLog),
	}, nil
}

func flattenKillLog(roundLog []models.RoundSnapshot) []string {
	var out []string
	for _, r := range roundLog {
		out = append(out, r.KillLog...)
	}
	return out
}

func computeDerivedStats(s *models.PlayerStats, tickRate float64) {
	// AverageSpeed needs total time, not just round time.
	// We store total distance; speed is computed in the result handler
	// once we know the full duration. For now, store the raw distance.
	_ = math.Round // imported for future use
}
