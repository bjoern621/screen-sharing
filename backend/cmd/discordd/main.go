// Command discordd keeps each Discord voice channel's group true (docs/discord-mode.md).
//
// A binary of its own because it is a different trust:
// it holds the bot token, the OAuth application secret and every session's group key,
// where groupd holds the signing key and the leases.
// It speaks to groupd as any member's app does, over the public routes,
// so groupd runs unchanged beside it.
//
// Secrets arrive in the environment rather than on the command line,
// argv being readable by every process on the machine:
// DISCORD_BOT_TOKEN, DISCORD_CLIENT_ID, DISCORD_CLIENT_SECRET.
package main

import (
	"flag"
	"net/http"
	"os"
	"time"

	"bjoernblessin.de/go-utils/util/logger"

	"bjoernblessin.de/screenshare/internal/channelgroup"
	"bjoernblessin.de/screenshare/internal/discordapi"
	"bjoernblessin.de/screenshare/internal/discordgateway"
	"bjoernblessin.de/screenshare/internal/discordoauth"
	"bjoernblessin.de/screenshare/internal/groupclient"
	"bjoernblessin.de/screenshare/internal/linkstore"
	"bjoernblessin.de/screenshare/internal/voiceroster"
)

// sweepEvery is how often empty channels are checked against the retire window.
// Well inside channelgroup.RetireAfter, so a retire lands close to the window it names.
const sweepEvery = 15 * time.Second

// version is the build stamp every answer names (internal/serving),
// which is what a member's relay check reads to say what this deployment is running.
//
// In main because that is what the linker writes into: -ldflags "-X main.version=...".
// "dev" answers for a build nobody stamped.
var version = "dev"

// main serves the manager on the address given.
//
// A missing secret, an unreadable link store and an unreachable gateway all end the process
// through logger.Errorf: each is an Umgebungsfehler,
// and a manager missing any of them can neither link nor enforce,
// so the alternative is a process that is up and refusing every request.
func main() {
	listen := flag.String("listen", "127.0.0.1:9444", "address to serve on")
	linksPath := flag.String("links", "discord-links.json", "file holding the durable links")
	groupService := flag.String("group-service", "http://127.0.0.1:9443", "where groupd answers")
	callbackURL := flag.String("callback-url", "", "public URL of GET /link/callback, as Discord redirects to it")
	flag.Parse()

	botToken := os.Getenv("DISCORD_BOT_TOKEN")
	clientID := os.Getenv("DISCORD_CLIENT_ID")
	clientSecret := os.Getenv("DISCORD_CLIENT_SECRET")
	if botToken == "" || clientID == "" || clientSecret == "" {
		logger.Errorf("DISCORD_BOT_TOKEN, DISCORD_CLIENT_ID and DISCORD_CLIENT_SECRET name the Discord application, and at least one is not set")
	}
	if *callbackURL == "" {
		logger.Errorf("-callback-url names where Discord sends a linking browser back, and it is not set")
	}

	links, err := linkstore.Open(*linksPath)
	if err != nil {
		logger.Errorf("%v", err)
	}

	broker := channelgroup.New(groupclient.New(), *groupService, links, time.Now)
	roster := voiceroster.New(broker.Leave)
	broker.ReadOccupancy(roster)

	gateway, err := discordgateway.Connect(botToken, roster)
	if err != nil {
		logger.Errorf("%v", err)
	}
	defer gateway.Close()

	go sweep(broker)

	oauth := discordoauth.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  *callbackURL,
	}
	service := discordapi.New(broker, links, oauth)
	logger.Infof("serving Discord mode on %s, groups at %s", *listen, *groupService)

	// Bounds as groupd serves under, the routes being the same small JSON over a local hop,
	// and the same proxy terminating TLS in front (deploy/Caddyfile).
	server := &http.Server{
		Addr:              *listen,
		Handler:           service.Handler(version),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	if err := server.ListenAndServe(); err != nil {
		logger.Errorf("serving: %v", err)
	}
}

// sweep retires empty channels for as long as this process runs,
// the manager's one timer (internal/channelgroup, Sweep).
func sweep(broker *channelgroup.Broker) {
	ticker := time.NewTicker(sweepEvery)
	defer ticker.Stop()

	for range ticker.C {
		broker.Sweep()
	}
}
