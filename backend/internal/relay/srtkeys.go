package relay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"bjoernblessin.de/go-utils/util/assert"
)

// SRT is keyed per prefix out of the relay's path configuration:
// an entry named "~^<prefix>" is a regex MediaMTX matches against every path under the prefix,
// and the passphrases inside key both directions of the handshake, measured against v1.20.0.
// The group service writes the entries through here,
// from the same derivation the app runs (internal/group, SrtPassphrase).

// srtKeysConf is the slice of a path configuration entry this app owns.
type srtKeysConf struct {
	SrtPublishPassphrase string `json:"srtPublishPassphrase"`
	SrtReadPassphrase    string `json:"srtReadPassphrase"`
}

// prefixAlphabet is every character a group prefix spells, the id encoding's own and the separator.
// None is a regex metacharacter,
// which is what lets a prefix stand inside an anchored regex unquoted.
const prefixAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567/"

// EnsureSRTKeys makes the relay key SRT under prefix with passphrase, both directions.
//
// Idempotent and read-through:
// the entry is read, written only where it differs, and nothing is held here.
// The relay's configuration is the one copy, so one restarted empty is re-seeded by the next call.
func (c *Client) EnsureSRTKeys(host string, apiPort int, prefix, passphrase string) error {
	assert.Assert(apiPort > 0, "apiPort comes from validated settings", apiPort)
	assert.Assert(strings.HasSuffix(prefix, "/"), "a prefix ends at the segment boundary permissions match on", prefix)
	assert.Assert(strings.Trim(prefix, prefixAlphabet) == "",
		"a prefix spells only the id alphabet, so it stands inside a regex unquoted", prefix)
	assert.Assert(passphrase != "", "a keyed prefix is keyed with something")

	client := c.httpClient()
	name := "~^" + prefix
	base := fmt.Sprintf("http://%s:%d/v3/config/paths", host, apiPort)
	want := srtKeysConf{SrtPublishPassphrase: passphrase, SrtReadPassphrase: passphrase}

	resp, err := client.Get(base + "/get/" + url.PathEscape(name))
	if err != nil {
		return fmt.Errorf("the relay did not answer for the path entry %s: %w", name, err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, listLimit))
	resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return writeConf(client, base+"/add/"+url.PathEscape(name), http.MethodPost, name, want)
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("the relay answered the path entry %s with %s", name, resp.Status)
	case readErr != nil:
		return fmt.Errorf("the relay's answer for the path entry %s broke off: %w", name, readErr)
	}

	var held srtKeysConf
	if err := json.Unmarshal(body, &held); err != nil {
		return fmt.Errorf("the relay's path entry %s does not read as one: %w", name, err)
	}
	if held == want {
		return nil
	}
	return writeConf(client, base+"/patch/"+url.PathEscape(name), http.MethodPatch, name, want)
}

// writeConf sends one configuration entry and carries a refusal out with the relay's own words.
func writeConf(client *http.Client, address, method, name string, conf srtKeysConf) error {
	encoded, err := json.Marshal(conf)
	assert.IsNil(err, "a path entry of two strings encodes")

	request, err := http.NewRequest(method, address, bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("building the write for the path entry %s: %w", name, err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("the relay did not answer a write of the path entry %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, listLimit))
		return fmt.Errorf("the relay refused the path entry %s: %s %s",
			name, resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}
