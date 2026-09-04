// Package channelgroup keeps each voice channel's group true at groupd.
//
// A voice channel is a group (docs/discord-mode.md).
// The broker owns the mapping: it draws a group for a channel's first member,
// holds the key and every member secret, and hands out derived facts alone,
// so no key ever reaches an app in this mode.
//
// Presence names the state one install wants true and answers the whole of it,
// idempotent as groupd's PUT /members is.
// It reaches groupd only where the roster confirms the channel,
// so a lease means standing in the channel with the app running, both.
//
// Everything here is session state, rebuilt from the gateway after a restart:
// leases lapse, streams close, and the next pass draws fresh groups.
package channelgroup

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/linkstore"
	"bjoernblessin.de/screenshare/internal/voiceroster"
)

// RetireAfter is how long a channel stays mapped after its last member left.
//
// Long enough that everyone dropping and rejoining keeps the session's group,
// short enough that a prefix outlives no session by much.
const RetireAfter = time.Minute

// ErrUnlinked refuses a secret the link store never drew.
var ErrUnlinked = errors.New("this link secret names no linked Discord account, so link Discord in the app again")

// ErrNoChannel refuses a trade for an account standing in no voice channel.
var ErrNoChannel = errors.New("this account stands in no voice channel, so there is no group to grant")

// GroupService is what the broker asks of groupd, the shape groupclient answers.
type GroupService interface {
	CreateGroup(base string) (groupKey, groupID string, err error)
	State(base, groupKey, memberSecret, displayName string) (groupclient.Membership, error)
	Release(base, groupKey, memberSecret string) error
	Streams(base, groupKey string) ([]groupclient.Stream, error)
	Token(base, groupKey, memberSecret string) (string, error)
}

// Occupancy is what the broker reads of the voice roster.
type Occupancy interface {
	Where(userID string) (voiceroster.Presence, bool)
	Occupants(guildID, channelID string) int
}

// Links resolves link secrets to the Discord accounts they name.
type Links interface {
	Resolve(secret string) (linkstore.Link, bool)
}

// Answer is one pass's whole truth for one install.
// Channel and Group are nil together, for an account standing in no voice channel.
type Answer struct {
	Channel *Channel
	Group   *Group
}

// Channel labels where the account stands, for a person to read.
type Channel struct {
	Guild string
	Name  string
}

// Group carries every fact the app derives from a group key in manual mode,
// derived here instead so the key stays on this side.
type Group struct {
	Prefix        string
	SrtPassphrase string
	MemberID      string
	DisplayName   string
	LeaseSeconds  int
	Members       []groupclient.Member
	// PublishingUnread crosses from groupd's answer as it stands (groupclient.Membership).
	PublishingUnread bool
	Streams          []groupclient.Stream
}

// place names one voice channel.
type place struct {
	guildID   string
	channelID string
}

// session is one channel's group as this broker holds it.
type session struct {
	key     group.Key
	encoded string
	// members by link secret: one install standing in this channel is one member.
	members map[string]*memberState
	// emptySince is when the sweep first found the channel empty, zero while occupied.
	emptySince time.Time
}

// memberState is one install's identity inside one session.
type memberState struct {
	userID string
	secret group.MemberSecret
	// displayName is the claim that landed, held because the first claim holds at groupd.
	displayName string
}

// Broker holds the sessions. Safe for concurrent use.
//
// One lock across every operation, groupd round trips included:
// the callers are one app poll per member and the gateway's events,
// a load a single file of sessions serves without contention worth designing for.
type Broker struct {
	groups GroupService
	base   string
	links  Links
	now    func() time.Time

	mu        sync.Mutex
	occupancy Occupancy
	sessions  map[place]*session
}

// New is a broker asking groups at base, resolving links there and reading the clock given.
// It reads no roster until ReadOccupancy hands it one,
// the roster's leave callback needing the broker first.
func New(groups GroupService, base string, links Links, now func() time.Time) *Broker {
	assert.IsNotNil(groups, "a broker speaks to a group service")
	assert.Assert(base != "", "a broker names the group service it speaks to")
	assert.IsNotNil(links, "a broker resolves link secrets somewhere")
	assert.IsNotNil(now, "a broker reads a clock, the retire window being measured on it")

	return &Broker{
		groups:   groups,
		base:     base,
		links:    links,
		now:      now,
		sessions: map[place]*session{},
	}
}

// ReadOccupancy points the broker at the roster it reads.
func (b *Broker) ReadOccupancy(o Occupancy) {
	assert.IsNotNil(o, "a broker reads who stands where from a roster")

	b.mu.Lock()
	defer b.mu.Unlock()
	b.occupancy = o
}

// Presence states one install's presence where the roster confirms it,
// and answers channel, group and streams as they stand.
//
// Idempotent: every pass names the same state and lands the same way.
// An account in no channel gets the empty answer,
// and whatever that install held elsewhere is released on the same pass,
// which is what catches a leave the gateway never delivered.
func (b *Broker) Presence(linkSecret string) (Answer, error) {
	link, ok := b.links.Resolve(linkSecret)
	if !ok {
		return Answer{}, ErrUnlinked
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	assert.IsNotNil(b.occupancy, "presence is confirmed against a roster")

	where, in := b.occupancy.Where(link.UserID)
	b.releaseStale(linkSecret, where, in)
	if !in {
		return Answer{}, nil
	}

	s, m, stated, err := b.ensureMember(linkSecret, link.UserID, where)
	if err != nil {
		return Answer{}, err
	}

	streams, err := b.groups.Streams(b.base, s.encoded)
	if err != nil {
		return Answer{}, err
	}

	answer := Answer{
		Channel: &Channel{Guild: where.GuildName, Name: where.ChannelName},
		Group: &Group{
			Prefix:           s.key.Prefix(),
			SrtPassphrase:    s.key.SrtPassphrase(),
			MemberID:         stated.MemberID,
			DisplayName:      m.displayName,
			LeaseSeconds:     stated.LeaseSeconds,
			Members:          stated.Members,
			PublishingUnread: stated.PublishingUnread,
			Streams:          streams,
		},
	}
	assert.Assert(answer.Group.Prefix != "" && answer.Group.MemberID != "",
		"an answer inside a channel carries the derived facts")
	return answer, nil
}

// ensureMember holds the channel's session and this install's member true,
// stating the presence at groupd, and answers what groupd answered.
// Caller holds mu.
func (b *Broker) ensureMember(linkSecret, userID string, where voiceroster.Presence) (*session, *memberState, groupclient.Membership, error) {
	at := place{guildID: where.GuildID, channelID: where.ChannelID}
	assert.Assert(at.guildID != "" && at.channelID != "", "a member stands in a named channel")

	s, held := b.sessions[at]
	if !held {
		encoded, _, err := b.groups.CreateGroup(b.base)
		if err != nil {
			return nil, nil, groupclient.Membership{}, err
		}
		key, err := group.ParseKey(encoded)
		if err != nil {
			return nil, nil, groupclient.Membership{},
				fmt.Errorf("the group service answered a key this broker cannot read: %w", err)
		}
		s = &session{key: key, encoded: encoded, members: map[string]*memberState{}}
		b.sessions[at] = s
	}
	// Occupied, whatever the sweep last thought.
	s.emptySince = time.Time{}

	m, held := s.members[linkSecret]
	if !held {
		secret, err := group.NewMemberSecret()
		if err != nil {
			return nil, nil, groupclient.Membership{}, err
		}
		m = &memberState{userID: userID, secret: secret}
		s.members[linkSecret] = m
	}

	stated, err := b.claim(s, m, where.DisplayName)
	if err != nil {
		return nil, nil, groupclient.Membership{}, err
	}
	return s, m, stated, nil
}

// claim states this member's presence under a name.
//
// The first pass claims the Discord name, suffixing past whoever claimed it first,
// and every later pass restates the name that landed:
// the first claim holds at groupd, so a nick change waits for the group to retire.
func (b *Broker) claim(s *session, m *memberState, desired string) (groupclient.Membership, error) {
	assert.Assert(desired != "" || m.displayName != "", "a member is listed under a name")

	if m.displayName != "" {
		return b.groups.State(b.base, s.encoded, m.secret.String(), m.displayName)
	}

	// Bounded by the links one user may hold times the members a channel takes,
	// in practice a second install or a namesake, so single digits.
	for attempt := 1; attempt <= LinksPerChannelNamesakes; attempt++ {
		name := desired
		if attempt > 1 {
			name = fmt.Sprintf("%s %d", desired, attempt)
		}
		stated, err := b.groups.State(b.base, s.encoded, m.secret.String(), name)
		if groupclient.NameTaken(err) {
			continue
		}
		if err != nil {
			return groupclient.Membership{}, err
		}
		m.displayName = name
		return stated, nil
	}
	return groupclient.Membership{},
		fmt.Errorf("every name from %q to %q is claimed in this group",
			desired, fmt.Sprintf("%s %d", desired, LinksPerChannelNamesakes))
}

// LinksPerChannelNamesakes bounds the suffixes claim tries before giving up.
const LinksPerChannelNamesakes = 9

// releaseStale releases whatever this install holds outside the channel it stands in,
// or everywhere where it stands in none.
// Caller holds mu.
func (b *Broker) releaseStale(linkSecret string, where voiceroster.Presence, in bool) {
	current := place{guildID: where.GuildID, channelID: where.ChannelID}
	for at, s := range b.sessions {
		if in && at == current {
			continue
		}
		m, held := s.members[linkSecret]
		if !held {
			continue
		}
		b.release(s, linkSecret, m)
	}
}

// release drops one member at groupd and forgets it here.
//
// A release the service would not take is left to the lease,
// which lapses on its own within groupd's sweep, so the failure costs seconds and nothing more.
// Caller holds mu.
func (b *Broker) release(s *session, linkSecret string, m *memberState) {
	if err := b.groups.Release(b.base, s.encoded, m.secret.String()); err == nil {
		delete(s.members, linkSecret)
	}
}
