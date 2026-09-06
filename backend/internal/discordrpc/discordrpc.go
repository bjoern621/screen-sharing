// Package discordrpc states an activity on the Discord client running beside this app.
//
// A desktop client serves a local socket for exactly this, and one handshake names the application
// the activity is drawn under (docs/discord-mode.md).
// Everything after the handshake is one command, SET_ACTIVITY, carrying the whole activity,
// so a repeat states what already holds.
//
// No account, no token and no route of Discord's HTTP API is reached from here.
// What the socket grants is the profile of whoever is signed in to that client,
// which is why the connection is opened where the app runs rather than at the manager.
package discordrpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
)

// socketsPerHost is how many Discord instances a login can serve at once,
// each numbering its socket from zero (dial_other.go).
const socketsPerHost = 10

// dialTimeout bounds one attempt at one socket.
// Both answers are local and immediate: a live client accepts at once,
// and a number nothing serves refuses at once.
const dialTimeout = 250 * time.Millisecond

// exchangeTimeout bounds a command and the answer to it.
// A client that stopped reading its own socket is a fact worth reporting,
// where waiting on it holds up the poll every other snapshot rides (internal/app).
const exchangeTimeout = 2 * time.Second

// pongBudget is how many heartbeats an exchange answers before giving up on its own answer.
// The client asks on its schedule, so one can land between a command and the answer to it.
const pongBudget = 4

// noClientRunning is the reason both dials answer with, and the whole of what a user can act on:
// the socket exists while a Discord client does.
const noClientRunning = "no Discord client is running on this machine, so there is nothing to state an activity on"

// Client is one connection to a Discord client, holding the activity that connection states.
//
// A method is safe on one goroutine, the poll loop's (internal/app, richpresence.go).
type Client struct {
	conn net.Conn
	// stated is the activity JSON this connection carries, empty before the first send.
	// Held because the socket answers no question about what it shows,
	// and a repeat would spend one of the five sends the window allows (throttle.go).
	stated []byte
	sends  window
	// nonce numbers the commands, each answer naming the command it belongs to.
	nonce int
}

// Connect opens a running Discord client's socket and names the application on it.
//
// application is the Discord application's id, as the manager answers it
// (internal/discordclient).
// The id is the manager's own,
// so the activity is drawn under the same application the bot and the link flow speak for.
//
// A machine with no Discord client running is the error a caller surfaces and goes on from.
func Connect(application string) (*Client, error) {
	assert.Assert(application != "", "a handshake names the application the activity is drawn under")

	conn, err := dial()
	if err != nil {
		return nil, err
	}

	c := &Client{conn: conn}
	if err := c.handshake(application); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// Close hangs up. Safe on a connection already closed.
func (c *Client) Close() error {
	assert.IsNotNil(c, "a close names a connection")

	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// SetActivity states the activity this connection carries.
//
// Idempotent: an activity the connection already states sends nothing.
// A send past the window Discord allows is skipped and stated again by the next call,
// which is why the caller names the activity it wants on every pass rather than on a change it
// spotted (throttle.go).
func (c *Client) SetActivity(a Activity) error {
	msg := a.message()
	return c.state(&msg)
}

// ClearActivity takes this app's activity off the profile.
// Idempotent, as SetActivity is.
func (c *Client) ClearActivity() error {
	return c.state(nil)
}

// state sends one activity, or clears where activity is nil.
func (c *Client) state(activity *activityMessage) error {
	if c.conn == nil {
		return fmt.Errorf("cannot state an activity: %s", noClientRunning)
	}

	// Marshalled before the comparison,
	// the JSON being what the connection carries and what an unchanged pass compares equal on.
	stated, err := json.Marshal(activity)
	if err != nil {
		return fmt.Errorf("cannot encode the activity: %w", err)
	}
	if bytes.Equal(stated, c.stated) {
		return nil
	}
	if !c.sends.take(time.Now()) {
		return nil
	}

	c.nonce++
	command, err := json.Marshal(setActivity{
		Cmd:   "SET_ACTIVITY",
		Nonce: strconv.Itoa(c.nonce),
		Args:  setActivityArgs{Pid: os.Getpid(), Activity: activity},
	})
	if err != nil {
		return fmt.Errorf("cannot encode the activity command: %w", err)
	}

	if err := c.exchange(opFrame, command); err != nil {
		return err
	}

	c.stated = stated
	return nil
}

// handshake states the protocol version and the application, and reads the client's answer.
func (c *Client) handshake(application string) error {
	payload, err := json.Marshal(struct {
		V        int    `json:"v"`
		ClientID string `json:"client_id"`
	}{V: 1, ClientID: application})
	if err != nil {
		return fmt.Errorf("cannot encode the handshake: %w", err)
	}

	// The answer is a DISPATCH of READY naming the signed-in account, which nothing here reads:
	// what the connection is for is stating, and who is signed in is Discord's to draw beside it.
	return c.exchange(opHandshake, payload)
}

// exchange sends one frame and reads the answer to it, refusing what the answer refuses.
//
// Both halves run under one deadline, so a client that stopped reading and one that stopped
// answering cost the same bounded wait.
func (c *Client) exchange(op uint32, payload []byte) error {
	assert.IsNotNil(c.conn, "an exchange runs on an open connection")

	if err := c.conn.SetDeadline(time.Now().Add(exchangeTimeout)); err != nil {
		return fmt.Errorf("cannot bound the exchange with the Discord client: %w", err)
	}
	if err := writeFrame(c.conn, op, payload); err != nil {
		return err
	}

	for range pongBudget {
		answerOp, answer, err := readFrame(c.conn)
		if err != nil {
			return err
		}

		switch answerOp {
		case opFrame:
			return refusalIn(answer)
		case opClose:
			return closedBy(answer)
		case opPing:
			if err := writeFrame(c.conn, opPong, answer); err != nil {
				return err
			}
		case opPong:
		case opHandshake:
			return fmt.Errorf("the Discord client answered a handshake to a command")
		default:
			return fmt.Errorf("the Discord client answered opcode %d, which this speaks nothing for", answerOp)
		}
	}
	return fmt.Errorf("the Discord client answered heartbeats and no answer to the command")
}

// refusalIn is the refusal an answer frame carries, and nil where it carries none.
func refusalIn(answer []byte) error {
	var frame struct {
		Evt  string `json:"evt"`
		Data struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(answer, &frame); err != nil {
		return fmt.Errorf("cannot read the Discord client's answer: %w", err)
	}
	if frame.Evt != "ERROR" {
		return nil
	}
	return fmt.Errorf("the Discord client refused the activity: %s (%d)", frame.Data.Message, frame.Data.Code)
}

// closedBy is the reason a close frame carries.
func closedBy(answer []byte) error {
	var frame struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(answer, &frame); err != nil {
		return fmt.Errorf("the Discord client hung up, and its reason did not decode: %w", err)
	}
	return fmt.Errorf("the Discord client hung up: %s (%d)", frame.Message, frame.Code)
}
