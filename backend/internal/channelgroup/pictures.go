package channelgroup

import (
	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/groupclient"
)

// A member id and a Discord account meet here and nowhere else.
// The group service lists members by id and by claimed name, the roster holds pictures by user,
// and the secrets that derive one id from one user are this broker's own.

// pictures is members carrying the picture the roster holds for each.
//
// A member the roster places nowhere carries none,
// which is where a leaver stands until the release lands.
// Caller holds mu.
func (b *Broker) pictures(s *session, members []groupclient.Member) []groupclient.Member {
	assert.IsNotNil(s, "pictures are read for the members of a session")
	assert.IsNotNil(b.occupancy, "a picture is read off the roster")

	byMember := make(map[string]string, len(s.members))
	for _, m := range s.members {
		where, in := b.occupancy.Where(m.userID)
		if !in {
			continue
		}
		byMember[s.key.MemberID(m.secret)] = where.AvatarURL
	}

	listed := len(members)
	for i := range members {
		members[i].AvatarURL = byMember[members[i].MemberID]
	}

	assert.Assert(len(members) == listed, "a picture names a member the group listed, and adds none",
		len(members), listed)
	return members
}
