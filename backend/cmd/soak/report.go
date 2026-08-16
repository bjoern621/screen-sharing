package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// One thing the probe found, as the line a triage pass reads.
//
// Kind is what failed, and the signature is what makes two runs of the same defect one entry: a
// soak reporting the same greying ten thousand times reports nothing.
type finding struct {
	At        string            `json:"at"`
	Kind      string            `json:"kind"`
	Signature string            `json:"signature"`
	Detail    string            `json:"detail"`
	Fields    map[string]string `json:"fields,omitempty"`
	Settings  json.RawMessage   `json:"settings,omitempty"`
	Seed      int64             `json:"seed"`
	Iteration int               `json:"iteration"`
}

type reporter struct {
	mu       sync.Mutex
	out      *os.File
	seen     map[string]int
	counts   map[string]int
	seed     int64
	iter     int
	checks   int
	verbose  bool
	maxPer   int
	started  time.Time
	lastLine time.Time
}

func newReporter(path string, seed int64, verbose bool) (*reporter, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &reporter{
		out:     file,
		seen:    map[string]int{},
		counts:  map[string]int{},
		seed:    seed,
		verbose: verbose,
		maxPer:  5,
		started: time.Now(),
	}, nil
}

func (r *reporter) setIteration(n int) {
	r.mu.Lock()
	r.iter = n
	r.mu.Unlock()
}

func (r *reporter) pass() {
	r.mu.Lock()
	r.checks++
	r.mu.Unlock()
}

// report writes a finding, unless this signature has already been written enough times to make the
// point.
func (r *reporter) report(kind, signature, detail string, fields map[string]string, settings proto.Message) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.checks++
	r.counts[kind]++
	r.seen[signature]++
	if r.seen[signature] > r.maxPer {
		return
	}

	entry := finding{
		At:        time.Now().Format(time.RFC3339),
		Kind:      kind,
		Signature: signature,
		Detail:    detail,
		Fields:    fields,
		Seed:      r.seed,
		Iteration: r.iter,
	}
	if settings != nil {
		raw, err := protojson.Marshal(settings)
		if err == nil {
			entry.Settings = raw
		}
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return
	}
	r.out.Write(append(line, '\n'))
	r.out.Sync()
	if r.verbose {
		fmt.Fprintf(os.Stderr, "FINDING %s %s: %s\n", kind, signature, detail)
	}
}

// progress is the line a supervisor reads to know the probe is alive and what it has cost.
func (r *reporter) progress(extra string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.lastLine) < 5*time.Second {
		return
	}
	r.lastLine = time.Now()
	fmt.Printf("[%s] iteration %d, %d checks, %d finding kinds%s\n",
		time.Since(r.started).Round(time.Second), r.iter, r.checks, len(r.counts), extra)
}

func (r *reporter) summary() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	kinds := make([]string, 0, len(r.counts))
	for kind := range r.counts {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool { return r.counts[kinds[i]] > r.counts[kinds[j]] })

	out := fmt.Sprintf("ran %s, %d iterations, %d checks\n", time.Since(r.started).Round(time.Second), r.iter, r.checks)
	if len(kinds) == 0 {
		return out + "no findings\n"
	}
	for _, kind := range kinds {
		out += fmt.Sprintf("  %-32s %d\n", kind, r.counts[kind])
	}
	return out
}

func (r *reporter) close() {
	r.out.Close()
}
