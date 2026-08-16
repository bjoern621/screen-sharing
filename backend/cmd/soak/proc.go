package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// One reading of a process tree: what it spent on the CPU, what it holds resident, and how long
// each GPU engine ran for it.
//
// The engine times are what tells a hardware encode from a software one on the machine rather than
// from the settings that asked for it.
// They are per process, so a second job loading the same GPU moves no figure here.
type treeSample struct {
	At      time.Time
	Pids    int
	CPUSec  float64
	RSSKiB  int64
	Threads int
	FDs     int
	// Descriptors of the root process alone, which is what a leak shows in: a child holds its own
	// and takes them with it.
	RootFDs  int
	EngineNs map[string]int64
	// Engine time of the processes below the root alone.
	PipelineNs map[string]int64
}

// Time between two samples, and what the tree spent in it.
type treeDelta struct {
	Wall       time.Duration
	CPUSec     float64
	EngineNs   map[string]int64
	PipelineNs map[string]int64
	RSSKiB     int64
	FDs        int
	Threads    int
}

var clockTicks = 100.0

// GPU engine time over every engine, which is what a hardware encode has and a software one has
// none of.
func (d treeDelta) engineTotal() int64 {
	var total int64
	for _, ns := range d.EngineNs {
		total += ns
	}
	return total
}

// Cores the tree held over the interval. 1.0 is one core saturated.
func (d treeDelta) cores() float64 {
	if d.Wall <= 0 {
		return 0
	}
	return d.CPUSec / d.Wall.Seconds()
}

func (d treeDelta) engineBusy() float64 {
	if d.Wall <= 0 {
		return 0
	}
	return float64(d.engineTotal()) / float64(d.Wall.Nanoseconds())
}

func diff(before, after treeSample) treeDelta {
	d := treeDelta{
		Wall:       after.At.Sub(before.At),
		CPUSec:     after.CPUSec - before.CPUSec,
		PipelineNs: map[string]int64{},
		EngineNs:   map[string]int64{},
		RSSKiB:     after.RSSKiB - before.RSSKiB,
		FDs:        after.FDs - before.FDs,
		Threads:    after.Threads - before.Threads,
	}
	for name, ns := range after.EngineNs {
		if delta := ns - before.EngineNs[name]; delta > 0 {
			d.EngineNs[name] = delta
		}
	}
	for name, ns := range after.PipelineNs {
		if delta := ns - before.PipelineNs[name]; delta > 0 {
			d.PipelineNs[name] = delta
		}
	}
	return d
}

// sampleTree reads one process and everything descended from it.
//
// A process that ended between the walk and the read contributes nothing rather than failing the
// sample: a publish child dying is the thing being measured.
func sampleTree(root int) treeSample {
	s := treeSample{At: time.Now(), EngineNs: map[string]int64{}, PipelineNs: map[string]int64{}}
	seen := map[uint64]bool{}
	for _, pid := range descendants(root) {
		stat, ok := readStat(pid)
		if !ok {
			continue
		}
		s.Pids++
		s.CPUSec += stat.cpuSec
		s.RSSKiB += stat.rssKiB
		s.Threads += stat.threads
		s.FDs += countFDs(pid)
		if pid == root {
			s.RootFDs = countFDs(pid)
		}

		engines := map[string]int64{}
		readEngines(pid, seen, engines)
		for name, ns := range engines {
			s.EngineNs[name] += ns
			// The backend decodes the broadcast preview in its own process, and that decode reaches
			// the same silicon an encode does. Only what runs below it is the pipeline's own work.
			if pid != root {
				s.PipelineNs[name] += ns
			}
		}
	}
	return s
}

// descendants is root and every process below it, root included even where it has none.
func descendants(root int) []int {
	children := map[int][]int{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []int{root}
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}
		stat, ok := readStat(pid)
		if !ok {
			continue
		}
		children[stat.ppid] = append(children[stat.ppid], pid)
	}

	out := []int{root}
	for i := 0; i < len(out); i++ {
		out = append(out, children[out[i]]...)
	}
	return out
}

type procStat struct {
	ppid    int
	cpuSec  float64
	rssKiB  int64
	threads int
}

// readStat parses /proc/<pid>/stat past the comm field, which may itself hold spaces and
// parentheses.
func readStat(pid int) (procStat, bool) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return procStat{}, false
	}
	line := string(data)
	close := strings.LastIndex(line, ")")
	if close < 0 || close+2 >= len(line) {
		return procStat{}, false
	}
	// Fields from state onward, so field N of proc(5) is at N-3.
	fields := strings.Fields(line[close+2:])
	at := func(n int) int64 {
		if n-3 >= len(fields) {
			return 0
		}
		v, _ := strconv.ParseInt(fields[n-3], 10, 64)
		return v
	}
	// Children that have been waited for are counted too: a probe that spawns an encoder and
	// reaps it would otherwise read as a process that spent nothing.
	return procStat{
		ppid:    int(at(4)),
		cpuSec:  float64(at(14)+at(15)+at(16)+at(17)) / clockTicks,
		rssKiB:  at(24) * int64(os.Getpagesize()) / 1024,
		threads: int(at(20)),
	}, true
}

func countFDs(pid int) int {
	entries, err := os.ReadDir("/proc/" + strconv.Itoa(pid) + "/fd")
	if err != nil {
		return 0
	}
	return len(entries)
}

// readEngines adds this process's DRM engine times to total.
//
// One GPU context appears under every descriptor that names it, so a client already counted is
// skipped: adding the same fdinfo twice would report several times the engine time actually spent.
// Key: "drm-engine-enc" under client 16544 on 0000:c3:00.0.
func readEngines(pid int, seen map[uint64]bool, total map[string]int64) {
	clients := map[uint64]map[string]int64{}
	readClients("/proc/"+strconv.Itoa(pid)+"/fdinfo", clients)
	for client, engines := range clients {
		if seen[client] {
			continue
		}
		seen[client] = true
		for name, ns := range engines {
			total[name] += ns
		}
	}
}

// readClients reads one process's DRM clients out of its descriptors, keyed so the same context is
// read once however many descriptors name it.
// Key: client 16544 on 0000:c3:00.0, holding "drm-engine-enc".
func readClients(dir string, out map[uint64]map[string]int64) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		file, err := os.Open(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var client string
		var pdev string
		engines := map[string]int64{}
		scan := bufio.NewScanner(file)
		for scan.Scan() {
			name, value, ok := strings.Cut(scan.Text(), ":")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			switch {
			case name == "drm-client-id":
				client = value
			case name == "drm-pdev":
				pdev = value
			case strings.HasPrefix(name, "drm-engine-"):
				ns, err := strconv.ParseInt(strings.TrimSuffix(value, " ns"), 10, 64)
				if err == nil {
					engines[name] = ns
				}
			}
		}
		file.Close()

		if client == "" || len(engines) == 0 {
			continue
		}
		out[hash(pdev+"/"+client)] = engines
	}
}

func hash(s string) uint64 {
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return h
}

func readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}
