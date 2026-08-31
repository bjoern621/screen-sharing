package relay

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The relay keys SRT per prefix out of its path configuration, so the group service writes each
// group's derived passphrase there (internal/groupsvc).
// These hold the exchange against MediaMTX v1.20.0's measured answers:
// 404 for an entry that is not there, 400 for an add that already is,
// and a regex entry name matched against every path under the prefix.

// configRelay serves a config API holding these entries and records every write.
type configRelay struct {
	entries map[string]srtKeysConf // key: entry name, "~^<prefix>"
	added   []string
	patched []string
}

func (c *configRelay) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		read := func() srtKeysConf {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("reading a write's body: %v", err)
			}
			var conf srtKeysConf
			if err := json.Unmarshal(body, &conf); err != nil {
				t.Fatalf("a write's body does not read as a path configuration: %v", err)
			}
			return conf
		}

		switch {
		case r.Method == http.MethodGet && len(r.URL.Path) > len("/v3/config/paths/get/"):
			name := r.URL.Path[len("/v3/config/paths/get/"):]
			conf, ok := c.entries[name]
			if !ok {
				http.Error(w, `{"status":"error","error":"path configuration not found"}`, http.StatusNotFound)
				return
			}
			if err := json.NewEncoder(w).Encode(conf); err != nil {
				t.Fatalf("encoding an entry: %v", err)
			}
		case r.Method == http.MethodPost && len(r.URL.Path) > len("/v3/config/paths/add/"):
			name := r.URL.Path[len("/v3/config/paths/add/"):]
			if _, ok := c.entries[name]; ok {
				http.Error(w, `{"status":"error","error":"path already exists"}`, http.StatusBadRequest)
				return
			}
			c.entries[name] = read()
			c.added = append(c.added, name)
		case r.Method == http.MethodPatch && len(r.URL.Path) > len("/v3/config/paths/patch/"):
			name := r.URL.Path[len("/v3/config/paths/patch/"):]
			if _, ok := c.entries[name]; !ok {
				http.Error(w, `{"status":"error","error":"path configuration not found"}`, http.StatusNotFound)
				return
			}
			c.entries[name] = read()
			c.patched = append(c.patched, name)
		default:
			http.NotFound(w, r)
		}
	})
}

// configServing is a client pointed at a relay holding these entries.
func configServing(t *testing.T, entries map[string]srtKeysConf) (*Client, *configRelay, string, int) {
	t.Helper()
	if entries == nil {
		entries = map[string]srtKeysConf{}
	}
	relayed := &configRelay{entries: entries}
	server := httptest.NewServer(relayed.handler(t))
	t.Cleanup(server.Close)
	host, port := hostPort(t, server.URL)
	return New(), relayed, host, port
}

const testPrefix = "ABCDEFGHIJKLMNOPQRSTUVWXY2/"

// A prefix the relay has never heard of is added, keyed in both directions:
// a passphrase on one side only is a stream that connects and never plays.
func TestAnUnknownPrefixIsAddedKeyedBothWays(t *testing.T) {
	client, relayed, host, port := configServing(t, nil)

	if err := client.EnsureSRTKeys(host, port, testPrefix, "a-derived-passphrase"); err != nil {
		t.Fatalf("keying a fresh prefix: %v", err)
	}

	entry, ok := relayed.entries["~^"+testPrefix]
	if !ok {
		t.Fatalf("no entry under the anchored regex name, entries: %v", relayed.entries)
	}
	if entry.SrtPublishPassphrase != "a-derived-passphrase" || entry.SrtReadPassphrase != "a-derived-passphrase" {
		t.Errorf("the entry keys %+v, want both directions on the derived passphrase", entry)
	}
}

// The state named already holds, so nothing is written:
// this runs on every token and every presence pass,
// and a write per pass would churn the relay's configuration for no change.
func TestAPrefixAlreadyKeyedIsLeftAlone(t *testing.T) {
	conf := srtKeysConf{SrtPublishPassphrase: "already-there", SrtReadPassphrase: "already-there"}
	client, relayed, host, port := configServing(t, map[string]srtKeysConf{"~^" + testPrefix: conf})

	if err := client.EnsureSRTKeys(host, port, testPrefix, "already-there"); err != nil {
		t.Fatalf("keying an already-keyed prefix: %v", err)
	}
	if len(relayed.added)+len(relayed.patched) != 0 {
		t.Errorf("an unchanged prefix was written: added %v, patched %v", relayed.added, relayed.patched)
	}
}

// An entry carrying another value is patched onto the derived one.
// The derivation is the one source, so whatever the entry held gives way to it.
func TestAPrefixKeyedOtherwiseIsPatched(t *testing.T) {
	conf := srtKeysConf{SrtPublishPassphrase: "an-old-value", SrtReadPassphrase: "an-old-value"}
	client, relayed, host, port := configServing(t, map[string]srtKeysConf{"~^" + testPrefix: conf})

	if err := client.EnsureSRTKeys(host, port, testPrefix, "the-derived-one"); err != nil {
		t.Fatalf("re-keying a prefix: %v", err)
	}
	if len(relayed.patched) != 1 {
		t.Fatalf("patched %v, want the one entry", relayed.patched)
	}
	if got := relayed.entries["~^"+testPrefix].SrtReadPassphrase; got != "the-derived-one" {
		t.Errorf("the entry reads %q after the patch, want the derived value", got)
	}
}

// A relay that will not answer is an Umgebungsfehler the caller hears about:
// swallowing it would report a keyed prefix whose first handshake fails.
func TestARelayThatRefusesIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	host, port := hostPort(t, server.URL)

	if err := New().EnsureSRTKeys(host, port, testPrefix, "a-derived-passphrase"); err == nil {
		t.Fatal("a refusing relay was reported as keyed")
	}
}
