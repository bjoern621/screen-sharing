package app

import (
	"errors"
	"fmt"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	screensharev1 "bjoernblessin.de/screenshare/api/gen/go/screenshare/v1"

	"bjoernblessin.de/screenshare/internal/control"
	"bjoernblessin.de/screenshare/internal/group"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/member"
	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/text"
	"bjoernblessin.de/screenshare/internal/wire"
)

// Membership is a lease this machine holds: stated on every pass of the relay poll,
// gone when it stops being stated (docs/plan.md).
//
// The identity file says this machine is in a group, so quitting keeps it and leaving deletes it.
// No timer of its own: the loop that already polls the relay is the heartbeat.
//
// membersMu orders presence, join and leave as one statement about one machine,
// so a pass in flight when a leave arrives lands before the release rather than after it.

// membership is the presence this machine last stated and the group the service answered with.
//
// A refusal is held rather than dropped: the relay closes a connection without stating a reason,
// so a stopped stream is read against this snapshot.
type membership struct {
	// Group is the id of the group the settings name.
	// Empty where none is set and where the stored key does not parse.
	Group string
	// Joined is whether this machine holds an identity in that group.
	Joined bool
	// Members is the group as the last presence the service took listed it, this machine included.
	Members []wire.Member
	// PublishingUnread is whether the list Publishing is read off would not answer,
	// so a member sending nothing and a member nothing could be read about look alike.
	PublishingUnread bool
	// Refusal is why the last statement of presence was not taken.
	// nil where it was taken, and where nothing reached the service to refuse it.
	Refusal *screensharev1.Text
	// Taken dates the last presence the service took, zero where none has been.
	Taken time.Time
	// Lease is how long that presence stands, as the service stated it.
	Lease time.Duration
	// Stale marks an answer a pass left standing because it did not reach the service,
	// and keeps an outage to one log line rather than one per pass.
	Stale bool
}

// lapsed reports presence the service took that nothing restated inside the lease it granted,
// the state the relay closes connections against.
//
// A lease the service stated nothing for measures nothing,
// so presence stands until the next pass replaces it.
func (m membership) lapsed() bool {
	return m.Joined && !m.Taken.IsZero() && m.Lease > 0 && time.Since(m.Taken) > m.Lease
}

// failure is what this machine's membership says about a child that stopped, nil where it says
// nothing.
//
// The relay states no reason of its own when it closes a connection,
// so presence last stated separates a membership the group stopped honouring from an ordinary drop.
// A cause is named only where membership is that reason:
// a missing element, a wrong passphrase and a relay that is down are the child's own to report,
// and a sentence about the group over any of them sends the reader where nothing is wrong.
// A refusal made on a ground no code here names travels whole rather than being restated,
// so whatever it carries reaches the reader.
func (m membership) failure() *screensharev1.Text {
	assert.Assert(m.Group != "" || !m.Joined, "a machine in a group names the group it is in")

	switch {
	case m.Group == "":
		return nil
	case !m.Joined:
		// A group key with no identity beside it buys a token as the group itself,
		// which the relay closes the moment any member states presence.
		// Joining draws one (docs/plan.md).
		return text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED)
	case m.Refusal.GetCode() == screensharev1.TextCode_TEXT_CODE_GROUP_SERVICE_REFUSED:
		return m.Refusal
	case m.Refusal != nil || m.lapsed():
		return text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_MEMBERSHIP_LAPSED)
	default:
		return nil
	}
}

func (m membership) snapshot() wire.MembersSnapshot {
	return wire.MembersSnapshot{
		Members:          m.Members,
		Refusal:          m.Refusal,
		Joined:           m.Joined,
		PublishingUnread: m.PublishingUnread,
	}
}

// membershipRefusal states why the group service did not take this machine's presence.
// nil where the service stated nothing,
// a service that could not be reached having refused nothing (standing).
//
// A name another member holds is the one ground this app has a code for,
// and the ground the relay acts on: no member id was claimed,
// so the connections this machine holds are closed.
// Every other stated ground is the service saying no in words no code here names,
// so its own words stay in the log rather than crossing as vocabulary
// (api/proto/screenshare/v1/text.proto).
func membershipRefusal(err error) *screensharev1.Text {
	assert.IsNotNil(err, "a refusal states what was refused")

	var refusal *groupclient.Refusal
	if !errors.As(err, &refusal) {
		return nil
	}
	if groupclient.NameTaken(err) {
		return text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_NAME_TAKEN)
	}
	return text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_SERVICE_REFUSED)
}

// membership is what the last pass of the presence loop landed, the zero value before one has run.
func (a *App) membership() membership {
	if last := a.members.Load(); last != nil {
		return *last
	}
	return membership{}
}

// setMembership records what the presence loop read, and nothing else writes it.
func (a *App) setMembership(m membership) {
	a.members.Store(&m)
}

// landMembership records the presence read and announces it,
// the one path a shell learns the group over.
func (a *App) landMembership(m membership) {
	a.setMembership(m)
	a.emit(wire.MembersStateEvent(m.snapshot()))
}

// MembersState is who this machine shares a group with, as the presence loop last read it.
func (a *App) MembersState() wire.MembersSnapshot {
	return a.membership().snapshot()
}

// statePresence states this machine's presence and announces the group the service answered with.
//
// One pass of the loop that already polls the relay:
// the lease covers many passes, and the ones that do not land cost nothing.
// A refusal lands too, being what a stopped stream is read against.
//
// The group is announced on every pass, as the relay snapshot beside it is:
// a shell draws from a whole state on a clock,
// where an event sent only on a change is one a reconnecting shell waits for.
func (a *App) statePresence() {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	a.membersMu.Lock()
	defer a.membersMu.Unlock()

	held := a.membership()
	next, why := a.membershipFor(s, held)

	// One line per outage rather than one per pass:
	// a service that is down would otherwise write a line into the session log every two seconds.
	switch {
	case why == nil:
	case next.Stale && !held.Stale:
		logger.Warnf("presence in this group was not restated, the answer already read standing until its lease runs out: %v", why)
	case !next.Stale && next.Refusal.GetCode() != held.Refusal.GetCode():
		logger.Warnf("presence in this group was not taken: %v", why)
	}
	a.landMembership(next)
}

// membershipFor is the membership one pass over s leaves standing,
// with the service's own words for the caller to log.
//
// held is what the pass before it left.
// A service this pass could not reach refused nothing,
// so the answer it last took stands under the lease that came with it,
// and lapses when that lease runs out: a 20 s lease survives nine passes that never landed.
// Everything met here is an Umgebungsfehler:
// an app whose presence nothing takes is one whose connections the relay closes,
// and the snapshot is what turns that close into a sentence.
// The words come back beside the snapshot because the statement carries a code,
// and a code alone does not name which relay would not answer.
func (a *App) membershipFor(s settings.Settings, held membership) (membership, error) {
	base, ok := s.Relay.GroupService()
	if !ok {
		return membership{}, nil
	}
	id, ok := groupID(s.Relay)
	if !ok {
		return membership{}, nil
	}
	identity, joined, err := member.Load(id)
	if err != nil {
		// Debug and not a warning: on the poll, a file that will not read fails on every pass.
		// Join and Leave carry the same failure to whoever asked for one, where a person meets it.
		logger.Debugf("the member identity for group %s is not readable: %v", id, err)
		return membership{Group: id}, nil
	}
	if !joined {
		// A group key with no identity: in the group's paths and not in the group.
		return membership{Group: id}, nil
	}
	if s.Relay.DisplayName == "" {
		// Refused here rather than at the service, which refuses an empty name too:
		// a round trip every two seconds buys nothing over a fact this side already holds.
		return membership{
			Group:   id,
			Joined:  true,
			Refusal: text.Of(screensharev1.TextCode_TEXT_CODE_GROUP_NAME_MISSING),
		}, errNameMissing
	}

	answer, err := a.groups.State(base, s.Relay.GroupKey, identity.Secret, s.Relay.DisplayName)
	if err != nil {
		if refusal := membershipRefusal(err); refusal != nil {
			return membership{Group: id, Joined: true, Refusal: refusal}, err
		}
		return standing(held, id), err
	}
	return presenceTaken(id, answer), nil
}

// standing is what a pass that did not reach the service leaves:
// the answer the service last took, under the lease that came with it, until that lease runs out.
//
// A group other than the one held here starts empty,
// rather than showing another group's members.
// A refusal for want of a name goes with it, this pass having read one.
func standing(held membership, id string) membership {
	assert.Assert(id != "", "a pass states presence in a group that names itself")

	if held.Group != id {
		return membership{Group: id, Joined: true, Stale: true}
	}
	held.Joined = true
	held.Stale = true
	if held.Refusal.GetCode() == screensharev1.TextCode_TEXT_CODE_GROUP_NAME_MISSING {
		held.Refusal = nil
	}
	return held
}

// The two states joining needs and cannot supply itself.
// A shell meets them as the contract's own refusals,
// decided off the same settings above the call (control/effects.go),
// and a caller reaching the backend directly gets these.
var (
	errNoGroup     = errors.New("a group is joined by its key, and none is set")
	errNameMissing = errors.New("joining a group takes a name this machine goes by, and none is set")
)

// presenceTaken is the group as the service answered it.
// Self is decided here against the member id the answer names for this machine,
// so no consumer has to hold one to draw its own row.
func presenceTaken(id string, answer groupclient.Membership) membership {
	assert.Assert(id != "", "presence is taken in a group that names itself")

	members := make([]wire.Member, 0, len(answer.Members))
	for _, m := range answer.Members {
		members = append(members, wire.Member{
			MemberID:    m.MemberID,
			DisplayName: m.DisplayName,
			Publishing:  m.Publishing,
			Self:        m.MemberID == answer.MemberID,
		})
	}

	assert.Assert(len(members) == len(answer.Members), "a row per member the group answered with",
		len(members), len(answer.Members))
	return membership{
		Group:            id,
		Joined:           true,
		Members:          members,
		PublishingUnread: answer.PublishingUnread,
		Taken:            time.Now(),
		Lease:            time.Duration(answer.LeaseSeconds) * time.Second,
	}
}

// JoinGroup draws this machine's identity in the group the settings name
// and states its presence at once.
//
// Idempotent: a group this machine is already in draws nothing,
// keeps the relay token minted on the identity it holds,
// and states the presence the loop states every pass anyway.
// The identity file states the name the group took,
// so it is written once the claim holds:
// a refused name would otherwise be what this machine states presence under on every later pass.
// A name another member holds is a Refused, carried as INVALID_ARGUMENT (control/refusal.go);
// an identity drawn by that same call goes with it,
// so nothing is left claiming a name this machine does not hold.
func (a *App) JoinGroup() error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	if _, ok := s.Relay.GroupService(); !ok {
		return fmt.Errorf("membership is stated at a group service, and this relay names none")
	}
	id, ok := groupID(s.Relay)
	if !ok {
		return errNoGroup
	}
	if s.Relay.DisplayName == "" {
		return errNameMissing
	}

	a.membersMu.Lock()
	defer a.membersMu.Unlock()

	identity, held, err := member.Load(id)
	if err != nil {
		return err
	}
	if !held {
		if identity, err = member.Join(id, s.Relay.DisplayName); err != nil {
			return err
		}
	}

	// The same statement the poll makes, over the identity that exists here.
	next, why := a.membershipFor(s, a.membership())
	if why != nil && !held {
		// Drawn by this call and not taken by the service.
		// Kept, it would leave this machine stating presence under a name it never claimed.
		if forget := member.Forget(id); forget != nil {
			logger.Warnf("the member identity drawn in group %s is not removed: %v", id, forget)
		}
		next = membership{Group: id}
	}
	a.landMembership(next)

	if why != nil {
		if groupclient.NameTaken(why) {
			return control.Refuse("%v", why)
		}
		return why
	}

	if identity.DisplayName != s.Relay.DisplayName {
		// The name the group took, written once the claim holds.
		if _, err := member.Join(id, s.Relay.DisplayName); err != nil {
			return err
		}
	}
	if !held {
		// The token this machine holds was minted before it had a member id,
		// so the next command trades again and names the secret drawn here.
		a.forgetRelayToken()
	}

	logger.Infof("joined the group %s as '%s'", id, s.Relay.DisplayName)
	return nil
}

// LeaveGroup releases this machine's presence and drops the identity it held,
// which the relay answers by closing what this machine had open in the group.
//
// Idempotent: a group this machine is not in is already the state the call names,
// so nothing is released and nothing fails.
// A release that did not reach the service leaves the identity in place,
// so asking again does the whole of it rather than half.
func (a *App) LeaveGroup() error {
	a.settingsMu.Lock()
	s := a.settings
	a.settingsMu.Unlock()

	a.membersMu.Lock()
	defer a.membersMu.Unlock()

	id, ok := groupID(s.Relay)
	if !ok {
		a.landMembership(membership{})
		return nil
	}
	identity, held, err := member.Load(id)
	if err != nil {
		return err
	}
	if !held {
		a.landMembership(membership{Group: id})
		return nil
	}

	base, ok := s.Relay.GroupService()
	if !ok {
		return fmt.Errorf("membership is released at a group service, and this relay names none")
	}
	if err := a.groups.Release(base, s.Relay.GroupKey, identity.Secret); err != nil {
		return err
	}
	if err := member.Forget(id); err != nil {
		return err
	}
	// The token this machine holds names a member id the group does not carry.
	a.forgetRelayToken()

	a.landMembership(membership{Group: id})
	logger.Infof("left the group %s", id)
	return nil
}

// groupID is the public digest of the group key the settings name, ok=false where none is set.
//
// A stored key that will not parse is an Umgebungsfehler and names no group,
// so this machine states no presence rather than stating it somewhere nobody is.
func groupID(r settings.Relay) (string, bool) {
	if r.GroupKey == "" {
		return "", false
	}
	groupKey, err := group.ParseKey(r.GroupKey)
	if err != nil {
		logger.Warnf("the stored group key does not parse, so this machine states no presence: %v", err)
		return "", false
	}
	return groupKey.ID(), true
}

// memberSecret is what this machine holds in the group the settings name.
// Empty where it holds nothing.
//
// A token minted on it names this machine's member id,
// the subject the relay's membership check closes a connection against.
// A group this machine has not joined names no member and trades on the group key alone,
// as a group being created does (docs/plan.md).
func (a *App) memberSecret(r settings.Relay) string {
	id, ok := groupID(r)
	if !ok {
		return ""
	}
	identity, held, err := member.Load(id)
	if err != nil {
		logger.Warnf("the member identity for group %s is not readable, so a relay token names no member: %v", id, err)
		return ""
	}
	if !held {
		return ""
	}
	return identity.Secret
}
