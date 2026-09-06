// Package discordapi is discordd's HTTP surface: the routes an app polls,
// and the two-legged browser flow that links an install to a Discord account.
//
// The refusal shape is groupd's, a sentence under "error",
// so the app's client reads both services the same way.
// Every route takes the link secret in the body,
// a secret in a URL being a secret in every proxy log (internal/groupsvc).
// The link flow is the exception and speaks browser:
// redirects out to Discord and back to loopback, and a plain page where it cannot.
package discordapi

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"bjoernblessin.de/go-utils/util/assert"

	"bjoernblessin.de/screenshare/internal/channelgroup"
	"bjoernblessin.de/screenshare/internal/discordoauth"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/serving"
)

// linkWindow is how long a started link waits for its callback.
// A person is clicking through one consent screen, so minutes cover it.
const linkWindow = 10 * time.Minute

// bodyLimit bounds a request body, a link secret being 44 characters of base64.
const bodyLimit = 4096

// Broker answers presence and trades tokens (internal/channelgroup).
type Broker interface {
	Presence(linkSecret string) (channelgroup.Answer, error)
	Token(linkSecret string) (token, prefix string, err error)
}

// Links draws a link for an identified account (internal/linkstore).
type Links interface {
	Draw(userID string) (string, error)
}

// OAuth is the Discord consent flow (internal/discordoauth).
type OAuth interface {
	AuthorizeURL(state string) string
	Identify(code string) (discordoauth.Identity, error)
	Application() string
}

// Service serves the routes. Safe for concurrent use.
type Service struct {
	broker Broker
	links  Links
	oauth  OAuth
	now    func() time.Time

	mu sync.Mutex
	// pending holds each started link by its state until the callback spends it.
	pending map[string]pendingLink
}

// pendingLink is one started link: where the secret lands, and when the start ages out.
type pendingLink struct {
	port    int
	started time.Time
}

// New is a service answering with this broker, drawing links there and consenting through that.
func New(broker Broker, links Links, oauth OAuth) *Service {
	assert.IsNotNil(broker, "a service answers presence off a broker")
	assert.IsNotNil(links, "a service draws links somewhere")
	assert.IsNotNil(oauth, "a service links through a consent flow")

	return &Service{
		broker: broker, links: links, oauth: oauth,
		now:     time.Now,
		pending: map[string]pendingLink{},
	}
}

// Handler is the service's routes, answering under the version given (internal/serving).
func (s *Service) Handler(version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /presence", s.statePresence)
	mux.HandleFunc("POST /tokens", s.issueToken)
	mux.HandleFunc("GET /link", s.startLink)
	mux.HandleFunc("GET /link/callback", s.finishLink)
	// What a relay check dials (internal/reach).
	// A route of its own because every other one here takes a credential or starts a consent flow,
	// so a check on one would either be refused or leave a pending link behind.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return serving.Naming("discordd", version, mux)
}

// wireAnswer is a presence answer as it crosses to the app.
//
// Application is the Discord application this manager links through,
// answered on every pass because the app's own Discord client asks for it by id
// (internal/discordrpc).
// One owner for it: the manager holds the credentials that application is,
// and an app carrying a copy would draw an activity under whichever id it was built with.
type wireAnswer struct {
	Application string       `json:"application"`
	Channel     *wireChannel `json:"channel"`
	Group       *wireGroup   `json:"group"`
}

type wireChannel struct {
	Guild string `json:"guild"`
	Name  string `json:"name"`
}

type wireGroup struct {
	Prefix           string               `json:"prefix"`
	SrtPassphrase    string               `json:"srtPassphrase"`
	MemberID         string               `json:"memberId"`
	DisplayName      string               `json:"displayName"`
	LeaseSeconds     int                  `json:"leaseSeconds"`
	Members          []groupclient.Member `json:"members"`
	PublishingUnread bool                 `json:"publishingUnread"`
	Streams          []groupclient.Stream `json:"streams"`
}

// statePresence relays one pass to the broker and answers the whole of it.
func (s *Service) statePresence(w http.ResponseWriter, r *http.Request) {
	secret, ok := linkSecretOf(w, r)
	if !ok {
		return
	}

	answer, err := s.broker.Presence(secret)
	if err != nil {
		refuseFor(w, err)
		return
	}

	wire := wireAnswer{Application: s.oauth.Application()}
	if answer.Channel != nil {
		wire.Channel = &wireChannel{Guild: answer.Channel.Guild, Name: answer.Channel.Name}
	}
	if answer.Group != nil {
		wire.Group = &wireGroup{
			Prefix:           answer.Group.Prefix,
			SrtPassphrase:    answer.Group.SrtPassphrase,
			MemberID:         answer.Group.MemberID,
			DisplayName:      answer.Group.DisplayName,
			LeaseSeconds:     answer.Group.LeaseSeconds,
			Members:          answer.Group.Members,
			PublishingUnread: answer.Group.PublishingUnread,
			Streams:          answer.Group.Streams,
		}
	}
	respond(w, wire)
}

// issueToken brokers one trade and answers token and prefix.
func (s *Service) issueToken(w http.ResponseWriter, r *http.Request) {
	secret, ok := linkSecretOf(w, r)
	if !ok {
		return
	}

	token, prefix, err := s.broker.Token(secret)
	if err != nil {
		refuseFor(w, err)
		return
	}
	respond(w, map[string]string{"relayAccessToken": token, "prefix": prefix})
}

// startLink records where the secret is to land and sends the browser to Discord.
//
// The state ties the callback to this start:
// drawn here, spent there, and aged out where no callback ever comes.
func (s *Service) startLink(w http.ResponseWriter, r *http.Request) {
	port, err := strconv.Atoi(r.URL.Query().Get("port"))
	if err != nil || port < 1 || port > 65535 {
		refuse(w, http.StatusBadRequest, "a link start names the loopback port the app listens on")
		return
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		refuse(w, http.StatusInternalServerError, "no link state could be drawn")
		return
	}
	state := base64.RawURLEncoding.EncodeToString(raw)

	s.mu.Lock()
	s.expire()
	s.pending[state] = pendingLink{port: port, started: s.now()}
	s.mu.Unlock()

	http.Redirect(w, r, s.oauth.AuthorizeURL(state), http.StatusFound)
}

// finishLink spends the callback's state, identifies the account and lands the secret on loopback.
//
// Failures answer a page a person reads in the browser tab Discord sent them back to,
// the app's own window knowing nothing of this leg.
func (s *Service) finishLink(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")

	s.mu.Lock()
	s.expire()
	link, ok := s.pending[state]
	delete(s.pending, state)
	s.mu.Unlock()
	if !ok {
		page(w, http.StatusBadRequest, "This link attempt is unknown or has expired. Start the link again from the app.")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		page(w, http.StatusBadRequest, "Discord sent no code back. Start the link again from the app.")
		return
	}

	identity, err := s.oauth.Identify(code)
	if err != nil {
		page(w, http.StatusBadGateway,
			fmt.Sprintf("Discord did not confirm the account (%v). Close this tab and start the link again from the app.", err))
		return
	}

	secret, err := s.links.Draw(identity.UserID)
	if err != nil {
		page(w, http.StatusInternalServerError,
			"The link could not be stored. Close this tab and start the link again from the app.")
		return
	}

	http.Redirect(w, r,
		fmt.Sprintf("http://127.0.0.1:%d/?linkSecret=%s", link.port, secret),
		http.StatusFound)
}

// expire drops every started link past its window. Caller holds mu.
func (s *Service) expire() {
	now := s.now()
	for state, link := range s.pending {
		if now.Sub(link.started) > linkWindow {
			delete(s.pending, state)
		}
	}
}

// linkSecretOf reads the secret a request names, and refuses the caller where it names none.
func linkSecretOf(w http.ResponseWriter, r *http.Request) (string, bool) {
	var body struct {
		LinkSecret string `json:"linkSecret"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, bodyLimit)).Decode(&body); err != nil {
		refuse(w, http.StatusBadRequest, "this request carries nothing this service can read")
		return "", false
	}
	if body.LinkSecret == "" {
		refuse(w, http.StatusUnauthorized, "a call names the link secret it acts for, and this one names none")
		return "", false
	}
	return body.LinkSecret, true
}

// refuseFor maps a broker's error to the status its ground takes.
//
// Unlinked is the caller's credential, no channel is the caller's state,
// and everything else is groupd or Discord not answering, which is this side's gateway to say.
func refuseFor(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, channelgroup.ErrUnlinked):
		refuse(w, http.StatusUnauthorized, err.Error())
	case errors.Is(err, channelgroup.ErrNoChannel):
		refuse(w, http.StatusBadRequest, err.Error())
	default:
		refuse(w, http.StatusBadGateway, err.Error())
	}
}

func respond(w http.ResponseWriter, body any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

// refuse answers in the shape groupd refuses in, a sentence under "error".
func refuse(w http.ResponseWriter, status int, reason string) {
	assert.Assert(status >= 400, "a refusal carries a failing status", status)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": reason})
}

// page answers a person's browser tab, plain text being what every browser renders.
func page(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintln(w, text)
}
