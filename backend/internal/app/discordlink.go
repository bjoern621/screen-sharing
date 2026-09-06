package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"bjoernblessin.de/go-utils/util/assert"
	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/settings"
	"bjoernblessin.de/screenshare/internal/wire"
)

// linkWindow bounds how long the flow waits for the browser leg to come back.
// A person is clicking through one consent screen, so minutes cover it,
// and the manager ages its half of the start out on the same order (internal/discordapi).
const linkWindow = 5 * time.Minute

// LinkDiscord links this install to a Discord account (docs/discord-mode.md).
//
// The backend runs the whole flow because both ends are its:
// the browser opens through the same shell call every player page takes,
// and the secret lands in the settings only this side writes.
// A loopback listener exists for the duration of one call and closes with it.
//
// Idempotent in effect: linking an install that is linked stores the fresh secret,
// which is the state the call names.
func (a *App) LinkDiscord(ctx context.Context, relay settings.Relay) error {
	base, ok := relay.DiscordService()
	if !ok {
		return errors.New("Discord is linked through the relay's manager, and no relay is named to reach one at")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("no loopback port for the link to land on: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	assert.Assert(port > 0, "a bound listener has a port", port)

	// Buffered, so the handler never blocks on a caller that already gave up.
	landed := make(chan string, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := r.URL.Query().Get("linkSecret")
		if secret == "" {
			linkPage(w, http.StatusBadRequest, "The link came back without a secret. Start the link again from the app.")
			return
		}
		linkPage(w, http.StatusOK, "Discord is linked. You can close this tab and return to the app.")
		select {
		case landed <- secret:
		default:
		}
	})}
	go server.Serve(listener)
	defer server.Close()

	if err := openInShell(base + "/link?port=" + strconv.Itoa(port)); err != nil {
		return fmt.Errorf("the browser did not open for the link: %v", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(linkWindow):
		return errors.New("nothing came back from the browser within five minutes: start the link again")
	case secret := <-landed:
		a.storeDiscordLink(secret)
		logger.Infof("Discord is linked")
		return nil
	}
}

// storeDiscordLink writes the one field the flow changes and announces the move,
// so a shell holding a draft re-reads rather than overwriting it on its next save.
//
// The Discord state goes out with it: a pass runs only in Discord mode (discord.go),
// so nothing else tells a shell that an install with the toggle still off is linked.
func (a *App) storeDiscordLink(secret string) {
	assert.Assert(secret != "", "a landed link carries its secret")

	a.settingsMu.Lock()
	a.settings.Relay.DiscordLink = secret
	s := a.settings
	a.settingsMu.Unlock()

	// A fresh secret has no pass behind it, and the answer standing was about the one it replaces:
	// a refusal recorded against that one says nothing about this.
	// The next pass lands the channel within one poll interval (watch.go).
	a.discordLast.Store(&discordSnapshot{})

	if err := settings.Save(s); err != nil {
		logger.Warnf("the link secret is not persisted, so this install unlinks on the next start: %v", err)
	}
	a.emit(wire.SettingsChangedEvent())
	a.emit(wire.DiscordStateEvent(a.discordWire()))
}

// linkPage answers the person's browser tab, plain text being what every browser renders.
func linkPage(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintln(w, text)
}
