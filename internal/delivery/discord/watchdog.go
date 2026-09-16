package discord

import (
	"errors"
	"log"
	"os"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// Slash commands and buttons reach the bot over the gateway websocket, and
// Discord gives it three seconds to answer. Messages the bot sends on its own —
// spawns, the daily ping — are plain REST calls that need no gateway at all. So
// when the gateway is gone the bot still looks alive in chat while every command
// ends in "didn't respond in time".
//
// That is exactly what happened: the network in front of this server dropped the
// gateway, discordgo logged a burst of failed handshakes and then went quiet —
// no connection, no further attempts — for days, while Telegram kept working.
// The library cannot be relied on to come back by itself: it holds its session
// lock across network reads that have no timeout, so a reconnect can stall
// forever without a word.
//
// So the connection is watched from outside. discordgo gets the first chance,
// and usually recovers within seconds. If the gateway stays down past the grace
// period, the watchdog closes and reopens the session itself, backing off so a
// long outage does not spend Discord's daily identify allowance. If a reopen
// never returns, the library is stuck in a way only a fresh process fixes: the
// bot exits, and Docker starts it again.
const (
	gatewayCheckEvery  = 30 * time.Second
	gatewayGracePeriod = 3 * time.Minute
	gatewayRetryFirst  = time.Minute
	gatewayRetryMax    = 10 * time.Minute
	gatewayReopenLimit = 2 * time.Minute
)

var errGatewayHung = errors.New("gateway reopen did not return")

type gatewayWatch struct {
	mu        sync.Mutex
	downSince time.Time // zero while connected
	everUp    bool
	nextTry   time.Time
	backoff   time.Duration
}

// watchGateway must run before the first Open, so the first READY is not missed.
func (b *Bot) watchGateway() {
	b.gw.mu.Lock()
	b.gw.downSince = time.Now() // not connected until the first READY
	b.gw.mu.Unlock()

	b.session.AddHandler(func(_ *discordgo.Session, _ *discordgo.Ready) { b.gatewayUp() })
	b.session.AddHandler(func(_ *discordgo.Session, _ *discordgo.Resumed) { b.gatewayUp() })
	b.session.AddHandler(func(_ *discordgo.Session, _ *discordgo.Disconnect) { b.gatewayDown() })

	go b.gatewayLoop()
}

func (b *Bot) gatewayUp() {
	b.gw.mu.Lock()
	defer b.gw.mu.Unlock()

	if !b.gw.downSince.IsZero() && b.gw.everUp {
		log.Printf("[DISCORD] gateway restored after %s", time.Since(b.gw.downSince).Round(time.Second))
	}
	b.gw.downSince = time.Time{}
	b.gw.everUp = true
	b.gw.backoff = 0
	b.gw.nextTry = time.Time{}
}

func (b *Bot) gatewayDown() {
	b.gw.mu.Lock()
	defer b.gw.mu.Unlock()

	// Closing the session to reopen it reports a disconnect too; the outage
	// started at the first one.
	if b.gw.downSince.IsZero() {
		b.gw.downSince = time.Now()
		log.Println("[DISCORD] gateway connection lost")
	}
}

func (b *Bot) gatewayLoop() {
	for range time.Tick(gatewayCheckEvery) {
		b.gw.mu.Lock()
		down := b.gw.downSince
		due := !time.Now().Before(b.gw.nextTry)
		b.gw.mu.Unlock()

		if down.IsZero() || time.Since(down) < gatewayGracePeriod || !due {
			continue
		}

		log.Printf("[DISCORD] gateway down for %s, reconnecting", time.Since(down).Round(time.Second))
		err := b.reopenGateway()
		switch {
		case errors.Is(err, errGatewayHung):
			log.Printf("[DISCORD] reconnect has not returned in %s — exiting so the container restarts", gatewayReopenLimit)
			os.Exit(1)
		case err != nil:
			b.gw.mu.Lock()
			b.gw.backoff = min(max(b.gw.backoff*2, gatewayRetryFirst), gatewayRetryMax)
			b.gw.nextTry = time.Now().Add(b.gw.backoff)
			log.Printf("[DISCORD] reconnect failed: %v (next try in %s)", err, b.gw.backoff)
			b.gw.mu.Unlock()
		default:
			b.gatewayUp()
			if !b.commandsSet.Load() {
				b.setupCommands()
			}
		}
	}
}

// reopenGateway closes the session and opens it again, and gives up waiting if
// that takes longer than any healthy connect does.
func (b *Bot) reopenGateway() error {
	done := make(chan error, 1)
	go func() {
		// Close first: a half-dead connection makes Open report "already open".
		_ = b.session.Close()
		done <- b.session.Open()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(gatewayReopenLimit):
		return errGatewayHung
	}
}
