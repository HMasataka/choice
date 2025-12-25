package sdpdebug

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pion/webrtc/v4"
)

// SaveAndLogSDP writes the SDP to a file and logs whether simulcast-related
// markers are present: a=simulcast, a=rid, a=ssrc-group:SIM. It is intended to
// be called right before SetRemoteDescription so you can verify what the client
// proposed, or anywhere you want to snapshot the SDP.
func SaveAndLogSDP(label string, sd webrtc.SessionDescription) {
	// Prepare output directory
	dir := "sdp_dumps"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		slog.Error("failed to create sdp dump dir", "error", err, "dir", dir)
		return
	}

	// Sanitize label for filename
	sanitized := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r
		case r >= '0' && r <= '9':
			return r
		case r == '-' || r == '_' || r == '.':
			return r
		default:
			return '-'
		}
	}, label)

	ts := time.Now().Format("20060102-150405.000")
	fname := fmt.Sprintf("%s_%s_%s.sdp", ts, sanitized, strings.ToLower(string(sd.Type)))
	path := filepath.Join(dir, fname)

	if err := os.WriteFile(path, []byte(sd.SDP), 0o644); err != nil {
		slog.Error("failed to write sdp dump", "error", err, "path", path)
		return
	}

	// Scan SDP for markers and log helpful lines
	hasSimulcast := false
	hasSIMGroup := false
	ridCount := 0
	var simulcastLines []string
	var ridLines []string
	var simGroupLines []string

	simulcastRe := regexp.MustCompile(`^a=simulcast:.*`)
	ridRe := regexp.MustCompile(`^a=rid:[^ ]+ +send|^a=rid:[^ ]+ +recv`)
	simGroupRe := regexp.MustCompile(`^a=ssrc-group:SIM .*`)

	scanner := bufio.NewScanner(strings.NewReader(sd.SDP))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case simulcastRe.MatchString(line):
			hasSimulcast = true
			simulcastLines = append(simulcastLines, line)
		case simGroupRe.MatchString(line):
			hasSIMGroup = true
			simGroupLines = append(simGroupLines, line)
		case ridRe.MatchString(line):
			ridCount++
			ridLines = append(ridLines, line)
		}
	}

	slog.Info("SDP dump saved",
		slog.String("path", path),
		slog.String("label", label),
		slog.String("type", string(sd.Type)),
		slog.Bool("has_simulcast_attr", hasSimulcast),
		slog.Int("rid_lines", ridCount),
		slog.Bool("has_ssrc_group_sim", hasSIMGroup),
	)

	if hasSimulcast {
		for _, l := range simulcastLines {
			slog.Info("sdp line", slog.String("simulcast", l))
		}
	}
	if ridCount > 0 {
		for _, l := range ridLines {
			slog.Info("sdp line", slog.String("rid", l))
		}
	}
	if hasSIMGroup {
		for _, l := range simGroupLines {
			slog.Info("sdp line", slog.String("ssrc-group-sim", l))
		}
	}
}
