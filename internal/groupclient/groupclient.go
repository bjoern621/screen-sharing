// Package groupclient is this app's side of the key, token and index service (internal/groupsvc).
//
// Three answers a member needs: the relay token a group key is worth, the streams that key can see,
// a fresh key where there is none.
//
// Every call reaches a service on another machine, so every failure is an Umgebungsfehler and
// leaves as an error.
// Unreachable, refused, malformed: conditions a user can act on, none of them this app's contract
// breaking.
//
// The token is the one fact kept between calls, the alternative being a round trip before every
// connection a viewer opens.
// It is kept beside what it was minted from, so a changed key mints again rather than reusing a
// credential for a group this app has left.
package groupclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// Refresh is how long before expiry a held token stops being handed out.
//
// It covers the gap between reading a token and the relay checking it: request, process start,
// handshake.
// The relay checks at the handshake and not again, so a token that survives that moment carries the
// whole connection (docs/plan.md).
const Refresh = 45 * time.Second

// Timeout bounds every call.
//
// Short: each call sits in front of something a user asked for, a publish starting or a list
// refreshing, and a service that is not answering should say so rather than hold the app.
const Timeout = 5 * time.Second

// Stream is one index entry: what the relay carries under the caller's prefix.
//
// Name is the stream's own inside the group, what a member picked when publishing,
// rather than the whole relay path.
// Format is the video track in the vocabulary the codec table keys on, which decides the transports
// a viewer may open it over.
type Stream struct {
	Name   string `json:"name"`
	Ready  bool   `json:"ready"`
	Tracks string `json:"tracks"`
	Format string `json:"format"`
}

// Client calls the service and holds the token it last minted.
// Safe for concurrent use: a poll and a publish start at once.
type Client struct {
	http *http.Client

	mu sync.Mutex
	// Token last minted, and what it was minted from, guarded by mu.
	// Not a keyed map: one group key is configured at a time, and a map would keep a token for every
	// group this app was ever pointed at.
	held    string
	expires time.Time
	from    origin
}

// origin is what a held token was minted from, compared against a later caller.
type origin struct {
	base string
	key  string
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: Timeout}}
}

// Token trades a group key for a relay token, handing back the held one while it has long enough
// left to open a connection with.
//
// The window is the service's and this asks for none.
// What a client decides is when to stop using a token before it expires, which is Refresh.
func (c *Client) Token(base, key string) (string, error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" || key == "" {
		return "", fmt.Errorf("a relay token is traded for a group key at a group service, and this relay has %s",
			missing(base, key))
	}

	now := time.Now()
	c.mu.Lock()
	held, expires, from := c.held, c.expires, c.from
	c.mu.Unlock()

	if held != "" && from == (origin{base: base, key: key}) && now.Add(Refresh).Before(expires) {
		return held, nil
	}

	var answer struct {
		Token   string `json:"token"`
		Prefix  string `json:"prefix"`
		Expires string `json:"expires"`
	}
	if err := c.post(base+"/tokens", map[string]string{"key": key}, &answer); err != nil {
		return "", err
	}
	if answer.Token == "" {
		return "", fmt.Errorf("the group service at %s answered with no token", base)
	}
	expiry, err := time.Parse(time.RFC3339, answer.Expires)
	if err != nil {
		return "", fmt.Errorf("the group service at %s dated its token %q, which is not a time: %v", base, answer.Expires, err)
	}

	c.mu.Lock()
	c.held, c.expires, c.from = answer.Token, expiry, origin{base: base, key: key}
	c.mu.Unlock()

	return answer.Token, nil
}

// Forget drops the held token, for a caller whose connection the relay refused.
// Without it a refused credential is handed out until it expires on its own.
func (c *Client) Forget() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.held, c.expires, c.from = "", time.Time{}, origin{}
}

// Streams is what the relay carries under this key's prefix, or the public streams without a key.
// The narrowing is the service's: a listing filtered here arrived carrying every group's streams.
func (c *Client) Streams(base, key string) ([]Stream, error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return nil, fmt.Errorf("a stream index is answered by a group service, and this relay names none")
	}

	address := base + "/streams"
	if key != "" {
		address += "?group=" + url.QueryEscape(key)
	}

	var answer struct {
		Prefix  string   `json:"prefix"`
		Streams []Stream `json:"streams"`
	}
	if err := c.get(address, &answer); err != nil {
		return nil, err
	}
	return answer.Streams, nil
}

// CreateKey draws a group key, which is the whole of creating a group: nothing is stored.
// Back comes the secret and the id its streams live under.
func (c *Client) CreateKey(base string) (key, id string, err error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return "", "", fmt.Errorf("a group key is drawn by a group service, and this relay names none")
	}

	var answer struct {
		Key string `json:"key"`
		ID  string `json:"id"`
	}
	if err := c.post(base+"/groups", map[string]string{}, &answer); err != nil {
		return "", "", err
	}
	if answer.Key == "" {
		return "", "", fmt.Errorf("the group service at %s answered with no key", base)
	}
	return answer.Key, answer.ID, nil
}

func (c *Client) post(address string, body any, into any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("rendering a request to %s: %v", address, err)
	}
	resp, err := c.http.Post(address, "application/json", strings.NewReader(string(encoded)))
	if err != nil {
		return unreachable(address, err)
	}
	defer resp.Body.Close()
	return read(address, resp, into)
}

func (c *Client) get(address string, into any) error {
	resp, err := c.http.Get(address)
	if err != nil {
		return unreachable(address, err)
	}
	defer resp.Body.Close()
	return read(address, resp, into)
}

// read decodes one answer into a value, or into the reason there is none.
// A refusal carries the service's own sentence and it passes through: restating it here is guessing
// at why.
func read(address string, resp *http.Response, into any) error {
	if resp.StatusCode >= 400 {
		var refusal struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&refusal) == nil && refusal.Error != "" {
			return fmt.Errorf("the group service refused: %s", refusal.Error)
		}
		return fmt.Errorf("the group service at %s answered %s", address, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("the group service at %s answered something this app cannot read: %v", address, err)
	}
	return nil
}

func unreachable(address string, err error) error {
	return fmt.Errorf("the group service at %s cannot be reached: %v", address, err)
}

// missing names which half of a token request is absent, so the message says what to set.
func missing(base, key string) string {
	switch {
	case base == "" && key == "":
		return "neither a group service nor a group key"
	case base == "":
		return "no group service, which is what a relay behind a TLS proxy has"
	default:
		return "no group key"
	}
}
