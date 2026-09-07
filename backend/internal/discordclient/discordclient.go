// Package discordclient is this app's side of the Discord manager (internal/discordapi).
//
// What Discord mode needs from it: the presence pass that answers channel,
// brokered group and streams in one round trip, and the token trade a command carries.
// Every call reaches a service on another machine,
// so every failure is an Umgebungsfehler and leaves as an error.
// A refusal travels as *groupclient.Refusal,
// the manager refusing in groupd's shape and this side reading both the same way.
package discordclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/groupclient"
)

// Timeout bounds every call, as groupclient's does and for the same reason:
// each sits in front of something a user asked for.
const Timeout = 5 * time.Second

// Channel labels where the linked account stands.
type Channel struct {
	Guild string `json:"guild"`
	Name  string `json:"name"`
}

// Group is the manager's stand-in for everything a group key would derive locally,
// plus the group and streams as groupd answered them on the same pass.
type Group struct {
	Prefix           string               `json:"prefix"`
	SrtPassphrase    string               `json:"srtPassphrase"`
	MemberID         string               `json:"memberId"`
	DisplayName      string               `json:"displayName"`
	LeaseSeconds     int                  `json:"leaseSeconds"`
	Members          []groupclient.Member `json:"members"`
	PublishingUnread bool                 `json:"publishingUnread"`
	Streams          []groupclient.Stream `json:"streams"`
}

// Answer is one pass's whole truth.
// Channel and Group are nil together, for an account standing in no voice channel.
//
// Application is the Discord application the manager links through, answered on every pass.
// An app states an activity on its own Discord client under that id (internal/discordrpc).
type Answer struct {
	Application string   `json:"application"`
	Channel     *Channel `json:"channel"`
	Group       *Group   `json:"group"`
}

// Client calls the manager. Safe for concurrent use, holding nothing between calls:
// the app's own pass snapshot is the one copy of the last answer.
type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: Timeout}}
}

// Presence states this install's presence and reads the whole answer.
// Idempotent end to end: the manager relays it as groupd's PUT /members.
func (c *Client) Presence(base, linkSecret string) (Answer, error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return Answer{}, errors.New("Discord mode is served by a manager, and no relay is named to reach one at")
	}

	var answer Answer
	if err := c.send(http.MethodPut, base+"/presence", linkSecret, &answer); err != nil {
		return Answer{}, err
	}
	return answer, nil
}

// Token trades the link secret for a relay access token under the current channel's prefix.
//
// Minted per call and cached nowhere:
// which group it grants follows the voice channel between two calls,
// and a held token would be a copy of a fact only the manager owns.
func (c *Client) Token(base, linkSecret string) (token, prefix string, err error) {
	assert.IsNotNil(c.http, "a client calls through a transport")

	if base == "" {
		return "", "", errors.New("a relay token is brokered by the manager, and no relay is named to reach one at")
	}

	var answer struct {
		Token  string `json:"relayAccessToken"`
		Prefix string `json:"prefix"`
	}
	if err := c.send(http.MethodPost, base+"/tokens", linkSecret, &answer); err != nil {
		return "", "", err
	}
	if answer.Token == "" {
		return "", "", fmt.Errorf("the manager at %s answered with no relay access token", base)
	}
	return answer.Token, answer.Prefix, nil
}

// send makes one call carrying the link secret and decodes its answer,
// or the reason there is none, in groupd's refusal shape.
func (c *Client) send(method, address, linkSecret string, into any) error {
	assert.Assert(address != "", "a call names the route it reaches", method)
	assert.IsNotNil(into, "a call names where its answer is read into", method, address)

	encoded, err := json.Marshal(map[string]string{"linkSecret": linkSecret})
	if err != nil {
		return fmt.Errorf("rendering a request to %s: %v", address, err)
	}
	request, err := http.NewRequest(method, address, strings.NewReader(string(encoded)))
	if err != nil {
		return fmt.Errorf("addressing %s: %v", address, err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(request)
	if err != nil {
		return fmt.Errorf("the manager at %s cannot be reached: %v", address, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var refusal struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(resp.Body).Decode(&refusal) == nil && refusal.Error != "" {
			return &groupclient.Refusal{Status: resp.StatusCode, Reason: refusal.Error}
		}
		return &groupclient.Refusal{
			Status: resp.StatusCode,
			Reason: fmt.Sprintf("%s answered %s", address, resp.Status),
		}
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("the manager at %s answered something this app cannot read: %v", address, err)
	}
	return nil
}
