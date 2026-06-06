package discord

import (
	"log"

	"github.com/bwmarrin/discordgo"
)

const platformDiscord = "discord"

// onGuildCreate registers a default posting channel for a guild, unless the
// guild already has one (GuildCreate fires on every reconnect, so we must not
// clobber an admin's /setmainchannel choice).
func (b *Bot) onGuildCreate(s *discordgo.Session, g *discordgo.GuildCreate) {
	if g.Unavailable {
		return // outage placeholder, not a real join
	}
	guildID := parseID(g.ID)

	has, err := b.repo.GuildHasChat(guildID)
	if err != nil {
		log.Printf("[DS chats] GuildHasChat(%d) failed: %v", guildID, err)
		return
	}
	if has {
		return
	}

	channelID := pickDefaultChannel(g.Guild)
	if channelID == 0 {
		log.Printf("[DS chats] no suitable text channel in guild %q (%s)", g.Name, g.ID)
		return
	}
	if err := b.repo.UpsertChatEnabled(platformDiscord, channelID, &guildID, g.Name); err != nil {
		log.Printf("[DS chats] register guild %q failed: %v", g.Name, err)
		return
	}
	log.Printf("[DS chats] registered guild %q default channel %d", g.Name, channelID)
}

// onGuildDelete disables spawns for a guild the bot was removed from. Ignores
// outage events (Unavailable), which are not real removals.
func (b *Bot) onGuildDelete(s *discordgo.Session, g *discordgo.GuildDelete) {
	if g.Unavailable {
		return
	}
	guildID := parseID(g.ID)
	if err := b.repo.DisableGuildChats(guildID); err != nil {
		log.Printf("[DS chats] disable guild %d failed: %v", guildID, err)
	} else {
		log.Printf("[DS chats] disabled guild %d (bot removed)", guildID)
	}
}

// pickDefaultChannel chooses a channel to post into: the system channel if set,
// otherwise the first text channel.
func pickDefaultChannel(g *discordgo.Guild) int64 {
	if g == nil {
		return 0
	}
	if g.SystemChannelID != "" {
		return parseID(g.SystemChannelID)
	}
	for _, c := range g.Channels {
		if c.Type == discordgo.ChannelTypeGuildText {
			return parseID(c.ID)
		}
	}
	return 0
}

// handleSetMainChannel marks the current channel as the guild's main channel
// (where the bot posts spawns/announcements). Requires Manage Server / Admin.
func (b *Bot) handleSetMainChannel(s *discordgo.Session, i *discordgo.InteractionCreate, lang string) {
	if i.GuildID == "" {
		b.respondEphemeral(s, i, b.loc.Translate(lang, "mainchannel_guild_only"))
		return
	}
	if i.Member == nil || i.Member.Permissions&(discordgo.PermissionManageGuild|discordgo.PermissionAdministrator) == 0 {
		b.respondEphemeral(s, i, b.loc.Translate(lang, "mainchannel_no_perms"))
		return
	}

	guildID := parseID(i.GuildID)
	channelID := parseID(i.ChannelID)

	title := ""
	if g, err := s.Guild(i.GuildID); err == nil && g != nil {
		title = g.Name
	}

	if err := b.repo.SetDiscordMainChannel(guildID, channelID, title); err != nil {
		log.Printf("[DS chats] set main channel failed: %v", err)
		b.respondEphemeral(s, i, b.loc.Translate(lang, "error_db"))
		return
	}
	b.respondEphemeral(s, i, b.loc.Translate(lang, "mainchannel_set"))
}
