package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SessionRecord holds the result of one practice session.
type SessionRecord struct {
	Date     time.Time `json:"date"`
	Mode     string    `json:"mode"`        // "check", "send", "shadow"
	Words    int       `json:"words"`       // total words attempted
	Correct  int       `json:"correct"`     // words answered correctly
	Retried  int       `json:"retried"`     // words where Delete was used at least once
	WPM      int       `json:"wpm"`         // character speed used
	Duration int64     `json:"duration_ms"` // session length in milliseconds
}

func statsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mt", "sessions.jsonl"), nil
}

func appendSession(rec SessionRecord) error {
	path, err := statsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func loadSessions() ([]SessionRecord, error) {
	path, err := statsPath()
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var sessions []SessionRecord
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		var rec SessionRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue // skip malformed lines
		}
		sessions = append(sessions, rec)
	}
	return sessions, sc.Err()
}

func printStats() {
	sessions, err := loadSessions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stats: %v\n", err)
		return
	}
	if len(sessions) == 0 {
		fmt.Println("No sessions recorded yet.")
		fmt.Println("Try: mt -check or mt -send")
		return
	}

	// Sort by date ascending.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Date.Before(sessions[j].Date)
	})

	// Aggregate totals.
	var totalWords, totalCorrect, totalRetried int
	modeCounts := map[string]int{}
	for _, s := range sessions {
		totalWords += s.Words
		totalCorrect += s.Correct
		totalRetried += s.Retried
		modeCounts[s.Mode]++
	}

	// Mode breakdown string.
	var modes []string
	for _, m := range []string{"check", "send", "shadow"} {
		if n := modeCounts[m]; n > 0 {
			label := m
			if m == "check" {
				label = "receive"
			}
			modes = append(modes, fmt.Sprintf("%d %s", n, label))
		}
	}
	modeStr := strings.Join(modes, ", ")
	if modeStr == "" {
		modeStr = "none"
	}

	fmt.Printf("Sessions: %d total (%s)\n", len(sessions), modeStr)

	if totalWords > 0 {
		fmt.Printf("Accuracy: %d%% overall (%d/%d correct)\n",
			100*totalCorrect/totalWords, totalCorrect, totalWords)
	}
	if totalRetried > 0 {
		fmt.Printf("Retried : %d %s used Delete\n",
			totalRetried, pluralize("word", "words", totalRetried))
	}

	// WPM trend: last up to 8 sessions.
	trend := sessions
	if len(trend) > 8 {
		trend = trend[len(trend)-8:]
	}
	if len(trend) > 1 {
		parts := make([]string, len(trend))
		for i, s := range trend {
			parts[i] = fmt.Sprintf("%d", s.WPM)
		}
		fmt.Printf("WPM     : %s (last %d)\n", strings.Join(parts, " → "), len(trend))
	}

	// Recent sessions (last 5, newest first).
	recent := sessions
	if len(recent) > 5 {
		recent = recent[len(recent)-5:]
	}
	fmt.Println("\nRecent sessions:")
	for i := len(recent) - 1; i >= 0; i-- {
		s := recent[i]
		pct := 0
		if s.Words > 0 {
			pct = 100 * s.Correct / s.Words
		}
		dur := time.Duration(s.Duration) * time.Millisecond
		retriedStr := ""
		if s.Retried > 0 {
			retriedStr = fmt.Sprintf("  %d retried", s.Retried)
		}
		fmt.Printf("  %s  %-6s  %d/%d (%3d%%)  %d wpm  %s%s\n",
			s.Date.Local().Format("2006-01-02"),
			s.Mode,
			s.Correct, s.Words, pct,
			s.WPM,
			formatDuration(dur),
			retriedStr,
		)
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func pluralize(singular, plural string, n int) string {
	if n == 1 {
		return singular
	}
	return plural
}
