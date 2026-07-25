package ffmpeg

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// parseProgress reads ffmpeg's -progress key=value stream and emits one Stats
// per block (blocks end with a "progress=" line). InstMbps is derived from the
// change in total_size and out_time between consecutive blocks.
func parseProgress(r io.Reader, onStats func(Stats)) {
	scanner := bufio.NewScanner(r)
	cur := map[string]string{}
	var prevBytes, prevTime float64
	havePrev := false

	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)

		if key != "progress" {
			cur[key] = value
			continue
		}

		bytesNow := parseFloat(cur["total_size"])
		timeNow := parseFloat(cur["out_time_us"]) / 1_000_000

		stats := Stats{
			Frame:   int(parseFloat(cur["frame"])),
			Fps:     parseFloat(cur["fps"]),
			SizeKiB: bytesNow / 1024,
			TimeSec: timeNow,
			Speed:   parseFloat(strings.TrimSuffix(cur["speed"], "x")),
			Drop:    int(parseFloat(cur["drop_frames"])),
			AvgMbps: parseFloat(strings.TrimSuffix(cur["bitrate"], "kbits/s")) / 1000,
		}
		if havePrev && timeNow > prevTime {
			stats.InstMbps = (bytesNow - prevBytes) * 8 / (timeNow - prevTime) / 1_000_000
		}
		prevBytes, prevTime, havePrev = bytesNow, timeNow, true

		onStats(stats)
		cur = map[string]string{}
	}
}

// parseFloat returns the float value of s, or 0 for "N/A" and unparseable input.
func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}
