// Package groupclient is this app's side of the key, token, membership and index service
// (internal/groupsvc).
//
// What a member needs from it: the relay access token a group key is worth, the streams that key can
// see, a fresh key where there is none, and its own presence in the group stated and released.
//
// Every call reaches a service on another machine, so every failure is an Umgebungsfehler and
// leaves as an error.
// Unreachable, refused, malformed: conditions a user can act on, none of them this app's contract
// breaking.
// A refusal keeps the service's own sentence and its status (Refusal), so a caller can tell a name
// another member holds from a service that is not answering.
//
// The token is the one fact kept between calls, the alternative being a round trip before every
// connection a viewer opens.
// It is kept beside what it was minted from, so a changed key, member or service mints again rather
// than reusing a credential for a group this app has left.
package groupclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Membership is what one statement of presence answers: this member, and the group it is in.
type Membership struct {
	MemberID    string `json:"memberId"`
	DisplayName string `json:"displayName"`
	// LeaseSeconds is how long the lease just stated holds, as the service sets it.
	// Read rather than assumed, so the refresh interval follows the service that grants it.
	LeaseSeconds int      `json:"leaseSeconds"`
	Members      []Member `json:"members"`
	// PublishingUnread is whether a connection list the service reads Publishing off would not answer.
	// Every member on such a list carries Publishing false, which is what a member sending nothing
	// carries too, so a reader shows the two differently.
	PublishingUnread bool `json:"publishingUnread"`
}

// Member is one live member of the group, this app included.
//
// Publishing is read off the relay's connection list on every answer, so a publish that dropped
// stops showing without a second call.
// False under PublishingUnread is a list that would not answer rather than a member sending nothing.
type Member struct {
	MemberID    string `json:"memberId"`
	DisplayName string `json:"displayName"`
	Publishing  bool   `json:"publishing"`
}

// Refusal is the group service's own answer to a request it would not take.
//
// The sentence passes through rather than being restated here, restating it being a guess at why:
// the service knows what it refused and this side does not.
// The status is what a caller acts on, a name another member holds being a request to make again
// under another name where every other refusal is not.
type Refusal struct {
	Status int
	Reason string
}

func (r *Refusal) Error() string {
	return "the group service refused: " + r.Reason
}

// NameTaken reports whether a display name another member holds is what this refusal names.
//
// Read off the status and not the sentence: 409 is what the service answers a claim on a live
// member's name, and matching prose would break on the first rewording (internal/groupsvc).
func NameTaken(err error) bool {
	var refusal *Refusal
	return errors.As(err, &refusal) && refusal.Status == http.StatusConflict
}

// Client calls the service and holds the token it last minted.
// Safe for concurrent use: a poll and a publish start at once.
type Client struct {
	http *http.Client

	mu sync.Mutex
	// Token last minted, and what it was minted from, guarded by mu.
	// Not a keyed map: one group is configured at a time, and a map would keep a token for every
	// group this app was ever pointed at.
	held    string
	expires time.Time
	from    origin
}

// origin is what a held token was minted from, compared against a later caller.
// The member secret is part of it because the token's subject derives from it: a token minted for
// another member names another subject, which is another connection at the relay.
type origin struct {
	base         string
	groupKey     string
	memberSecret string
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: Timeout}}
}

// Token trades a group key and a member secret for a relay access token, handing back the held one
// while it has long enough left to open a connection with.
//
// An empty group key is a request for the public prefix rather than a missing argument, so it is
// sent like any other: the service answers a token granting the streams anybody may watch, and a
// publisher in no group gets one instead of a refusal it could not act on.
// Only the service's address is required, and a relay nobody named has nowhere to ask.
//
// The window is the service's and this asks for none.
// What a client decides is when to stop using a token before it expires, which is Refresh.
func (c *Client) Token(base, groupKey, memberSecret string) (string, error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return "", errors.New("a relay token is issued by a group service, and no relay is named to ask one of")
	}

	now := time.Now()
	minted := origin{base: base, groupKey: groupKey, memberSecret: memberSecret}

	c.mu.Lock()
	held, expires, from := c.held, c.expires, c.from
	c.mu.Unlock()

	if held != "" && from == minted && now.Add(Refresh).Before(expires) {
		return held, nil
	}

	var answer struct {
		Token   string `json:"relayAccessToken"`
		Prefix  string `json:"prefix"`
		Expires string `json:"expires"`
	}
	body := map[string]string{"groupKey": groupKey, "memberSecret": memberSecret}
	if err := c.send(http.MethodPost, base+"/tokens", body, &answer); err != nil {
		return "", err
	}
	if answer.Token == "" {
		return "", fmt.Errorf("the group service at %s answered with no relay access token", base)
	}
	expiry, err := time.Parse(time.RFC3339, answer.Expires)
	if err != nil {
		return "", fmt.Errorf("the group service at %s dated its token %q, which is not a time: %v", base, answer.Expires, err)
	}

	c.mu.Lock()
	c.held, c.expires, c.from = answer.Token, expiry, minted
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
func (c *Client) Streams(base, groupKey string) ([]Stream, error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return nil, errors.New("a stream index is answered by a group service, and no relay is named to ask one of")
	}

	address := base + "/streams"
	if groupKey != "" {
		address += "?groupKey=" + url.QueryEscape(groupKey)
	}

	var answer struct {
		Prefix  string   `json:"prefix"`
		Streams []Stream `json:"streams"`
	}
	if err := c.send(http.MethodGet, address, nil, &answer); err != nil {
		return nil, err
	}
	return answer.Streams, nil
}

// CreateGroup draws a group key, which is the whole of creating a group: nothing is stored.
// Back comes the secret and the id its streams live under.
func (c *Client) CreateGroup(base string) (groupKey, groupID string, err error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return "", "", errors.New("a group key is drawn by a group service, and no relay is named to ask one of")
	}

	var answer struct {
		GroupKey string `json:"groupKey"`
		GroupID  string `json:"groupId"`
	}
	if err := c.send(http.MethodPost, base+"/groups", map[string]string{}, &answer); err != nil {
		return "", "", err
	}
	if answer.GroupKey == "" {
		return "", "", fmt.Errorf("the group service at %s answered with no group key", base)
	}
	return answer.GroupKey, answer.GroupID, nil
}

// State claims this member's presence in the group and refreshes it, both being the same call, and
// answers who else is there.
//
// Idempotent by construction: it names the state it wants true, that this member is here under this
// name, so it is what a poll sends on every pass.
//
// The display name is sent as it stands, an empty one included.
// The service refuses that with a sentence of its own, which is the reason a reader acts on, where
// a name invented here would join the group as somebody nobody chose to be.
func (c *Client) State(base, groupKey, memberSecret, displayName string) (Membership, error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return Membership{}, errors.New("membership is held by a group service, and no relay is named to state it to")
	}

	var answer Membership
	body := map[string]string{
		"groupKey":     groupKey,
		"memberSecret": memberSecret,
		"displayName":  displayName,
	}
	if err := c.send(http.MethodPut, base+"/members", body, &answer); err != nil {
		return Membership{}, err
	}
	if answer.MemberID == "" {
		return Membership{}, fmt.Errorf("the group service at %s stated no member for this secret", base)
	}
	return answer, nil
}

// Release drops this member's presence, which closes what it held at the relay.
//
// Idempotent: a member holding no lease is already in the state this names, so the service answers
// that there was none to release and this succeeds.
func (c *Client) Release(base, groupKey, memberSecret string) error {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return errors.New("membership is held by a group service, and no relay is named to release it at")
	}

	// The answer names whether there was a lease to release, and nothing here reads it: the state this
	// call names is that the member holds none, which is true either way.
	var answer struct {
		MemberID string `json:"memberId"`
		Released bool   `json:"released"`
	}
	body := map[string]string{"groupKey": groupKey, "memberSecret": memberSecret}
	return c.send(http.MethodDelete, base+"/members", body, &answer)
}

// send makes one call and decodes its answer, or the reason there is none.
// A nil body is a request that carries none, which is what a GET is.
func (c *Client) send(method, address string, body any, into any) error {
	assert.Assert(address != "", "a call names the route it reaches", method)
	assert.IsNotNil(into, "a call names where its answer is read into", method, spoken(address))

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("rendering a request to %s: %v", spoken(address), err)
		}
		payload = strings.NewReader(string(encoded))
	}

	request, err := http.NewRequest(method, address, payload)
	if err != nil {
		return fmt.Errorf("addressing %s: %v", spoken(address), err)
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(request)
	if err != nil {
		return unreachable(address, err)
	}
	defer resp.Body.Close()
	return read(address, resp, into)
}

// spoken is address with its query dropped, for a sentence a reader sees.
//
// The group key rides the index read as a query parameter, and holding it is what lets anybody join.
// Every error text this app shows is selectable so a reader can carry it into a bug report, which
// is exactly what would carry the key out with it, so the query is cut before the address is
// named rather than at each site that names one.
// The path survives, which is what tells one route from another.
func spoken(address string) string {
	if cut := strings.IndexByte(address, '?'); cut >= 0 {
		return address[:cut]
	}
	return address
}

// read decodes one answer into a value, or into the reason there is none.
//
// A refusal leaves as a *Refusal carrying what the service said and the status it said it under.
// Where the service named no reason, the status line and the route stand in for one: something
// answered and would not take the request, and which route it was is what a reader needs next.
func read(address string, resp *http.Response, into any) error {
	if resp.StatusCode >= 400 {
		var refusal struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&refusal) == nil && refusal.Error != "" {
			return &Refusal{Status: resp.StatusCode, Reason: refusal.Error}
		}
		return &Refusal{
			Status: resp.StatusCode,
			Reason: fmt.Sprintf("%s answered %s", spoken(address), resp.Status),
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("the group service at %s answered something this app cannot read: %v", spoken(address), err)
	}
	return nil
}

// unreachable names the route and the cause, and never the address the transport failed on.
//
// net/http wraps every failure in a *url.Error carrying the whole address, query and all, so the
// group key would ride out of the app inside a sentence spoken() had already cut it from.
// What is left of that wrapper is the cause itself, which is what says whether the service refused
// the connection, timed out or resolved to nothing.
func unreachable(address string, err error) error {
	var addressed *url.Error
	if errors.As(err, &addressed) {
		err = addressed.Err
	}
	return fmt.Errorf("the group service at %s cannot be reached: %v", spoken(address), err)
}
