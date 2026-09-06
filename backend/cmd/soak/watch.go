package main

import (
	"strconv"
	"sync"
	"time"
)

// engineWatcher accumulates GPU engine time while an operation runs.
//
// A before-and-after reading cannot see it: DRM engine counters live on the process,
// and a probe that spawns an encoder and reaps it takes them with it,
// unlike CPU time, which rolls up into the parent.
// So the counters are read on an interval,
// and the highest value each client reached is what it did.
//
// Counters are per client and monotonic, so the work one did is its last reading less its first.
type engineWatcher struct {
	root int

	mu      sync.Mutex
	first   map[string]int64
	last    map[string]int64
	ofRoot  map[uint64]bool
	stopped chan struct{}
	done    chan struct{}
}

func watchEngines(root int) *engineWatcher {
	w := &engineWatcher{
		root:    root,
		first:   map[string]int64{},
		last:    map[string]int64{},
		ofRoot:  map[uint64]bool{},
		stopped: make(chan struct{}),
		done:    make(chan struct{}),
	}
	go w.run()
	return w
}

func (w *engineWatcher) run() {
	defer close(w.done)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	w.read()
	for {
		select {
		case <-w.stopped:
			w.read()
			return
		case <-ticker.C:
			w.read()
		}
	}
}

// read takes one reading, keyed by client so a process that ends keeps what it did.
func (w *engineWatcher) read() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, pid := range descendants(w.root) {
		dir := "/proc/" + strconv.Itoa(pid) + "/fdinfo"
		clients := map[uint64]map[string]int64{}
		readClients(dir, clients)
		for client, engines := range clients {
			if pid == w.root {
				w.ofRoot[client] = true
			}
			for name, ns := range engines {
				key := strconv.FormatUint(client, 10) + "/" + name
				if _, ok := w.first[key]; !ok {
					w.first[key] = ns
				}
				if ns > w.last[key] {
					w.last[key] = ns
				}
			}
		}
	}
}

// stop ends the watch and answers what the pipelines below the root spent, per engine.
//
// The root's own clients are left out:
// the backend decodes the insights preview in its own process,
// and that decode reaches the same silicon an encode does.
func (w *engineWatcher) stop() map[string]int64 {
	close(w.stopped)
	<-w.done

	w.mu.Lock()
	defer w.mu.Unlock()
	out := map[string]int64{}
	for key, last := range w.last {
		client, name, ok := cut(key)
		if !ok || w.ofRoot[client] {
			continue
		}
		if spent := last - w.first[key]; spent > 0 {
			out[name] += spent
		}
	}
	return out
}

func cut(key string) (uint64, string, bool) {
	for i := 0; i < len(key); i++ {
		if key[i] == '/' {
			client, err := strconv.ParseUint(key[:i], 10, 64)
			return client, key[i+1:], err == nil
		}
	}
	return 0, "", false
}
