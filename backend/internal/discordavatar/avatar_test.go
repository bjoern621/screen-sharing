package discordavatar_test

import (
	"testing"

	"bjoernblessin.de/screenshare/internal/discordavatar"
)

func TestAnAccountsOwnPictureIsAddressedByItsHash(t *testing.T) {
	got := discordavatar.User("112233445566778899", "abc123")

	want := "https://cdn.discordapp.com/avatars/112233445566778899/abc123.png?size=64"
	if got != want {
		t.Fatalf("the account's picture is at %q, want %q", got, want)
	}
}

func TestAGuildPictureOutranksTheAccountsOwn(t *testing.T) {
	got := discordavatar.GuildMember("999", "112233445566778899", "def456")

	want := "https://cdn.discordapp.com/guilds/999/users/112233445566778899/avatars/def456.png?size=64"
	if got != want {
		t.Fatalf("the guild picture is at %q, want %q", got, want)
	}
}

func TestAnAccountWithNoPictureIsAddressedByItsId(t *testing.T) {
	// (id >> 22) % 6 is the index Discord draws a default under.
	got := discordavatar.User("739917264269541377", "")

	want := "https://cdn.discordapp.com/embed/avatars/3.png?size=64"
	if got != want {
		t.Fatalf("the default picture is at %q, want %q", got, want)
	}
}

func TestAnIdThatIsNoSnowflakeAddressesNothing(t *testing.T) {
	if got := discordavatar.User("not-a-snowflake", ""); got != "" {
		t.Fatalf("an unreadable id addresses %q, want no picture", got)
	}
}

func TestNoUserIsNoPicture(t *testing.T) {
	if got := discordavatar.User("", "abc123"); got != "" {
		t.Fatalf("an unnamed account addresses %q, want no picture", got)
	}
}
