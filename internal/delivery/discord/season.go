package discord

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"gachabot/internal/models"
	"gachabot/internal/service/season"

	"github.com/bwmarrin/discordgo"
)

// Seasons in Discord. Same words as Telegram — the text comes from the season
// service — wrapped in an embed, with buttons that swap the shelf for one medal
// in the message that is already there.

const (
	seasonColor  = 0x8b5cf6
	trophyColor  = 0xf1c40f
	trophyPrefix = "trophy:"
)

func (b *Bot) handleSeason(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User) {
	cur := b.season.Current()
	if cur == nil {
		b.respond(s, i, "Сезон сейчас не идёт. Когда он начнётся, здесь появится счёт.")
		return
	}

	rows, _, err := b.season.Scoreboard()
	if err != nil {
		log.Printf("[SEASON] не удалось прочитать счёт: %v", err)
		b.respond(s, i, "Не получилось прочитать счёт сезона, попробуй позже.")
		return
	}
	b.season.Decorate(rows)
	elapsed, _ := b.season.Days()

	embed := &discordgo.MessageEmbed{
		Description: season.BoardText(cur.Number, cur.Title, elapsed, b.season.DaysLeft(), rows, dbUser.ID),
		Color:       seasonColor,
	}

	components := []discordgo.MessageComponent{discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{Label: "Мои трофеи", Style: discordgo.SecondaryButton, CustomID: trophyData(0, dbUser.ID), Emoji: &discordgo.ComponentEmoji{Name: "🏆"}},
			b.launchAppButton(),
		},
	}}

	b.respondEmbedWithComponents(s, i, embed, components, isComponent(i))
}

func (b *Bot) handleTrophies(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, name string) {
	embed, components, err := b.shelfView(dbUser.ID, name)
	if err != nil {
		b.respond(s, i, "Не получилось прочитать трофеи, попробуй позже.")
		return
	}
	b.respondEmbedWithComponents(s, i, embed, components, isComponent(i))
}

// handleTrophyComponent switches the message between the shelf and one medal.
// CustomID is "trophy:<seasonNumber>", where 0 is the shelf itself.
func (b *Bot) handleTrophyComponent(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, name, raw string) {
	number, owner := parseTrophyData(raw)

	// A channel message is shared, so a button press from someone else must not
	// redraw it as their shelf.
	if owner != 0 && owner != dbUser.ID {
		b.respondEphemeral(s, i, "Это чужие трофеи. Свои открой командой /trophies")
		return
	}

	if number == 0 {
		embed, components, err := b.shelfView(dbUser.ID, name)
		if err != nil {
			return
		}
		b.respondEmbedWithComponents(s, i, embed, components, true)
		return
	}

	trophies, _, err := b.season.Trophies(dbUser.ID)
	if err != nil {
		return
	}
	for _, t := range trophies {
		if t.SeasonNumber != number {
			continue
		}
		embed := &discordgo.MessageEmbed{Description: season.TrophyText(t), Color: trophyColor}
		components := []discordgo.MessageComponent{discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{Label: "Все трофеи", Style: discordgo.SecondaryButton, CustomID: trophyData(0, dbUser.ID)},
				b.launchAppButton(),
			},
		}}
		b.respondEmbedWithComponents(s, i, embed, components, true)
		return
	}
}

// shelfView builds the shelf embed: the medals as fields, one button each.
func (b *Bot) shelfView(userID int64, name string) (*discordgo.MessageEmbed, []discordgo.MessageComponent, error) {
	trophies, _, err := b.season.Trophies(userID)
	if err != nil {
		log.Printf("[SEASON] не удалось прочитать трофеи игрока %d: %v", userID, err)
		return nil, nil, err
	}
	badges, err := b.season.Badges([]int64{userID})
	if err != nil {
		log.Printf("[SEASON] не удалось прочитать значок игрока %d: %v", userID, err)
	}
	badge := ""
	if bdg, ok := badges[userID]; ok {
		badge = season.BadgeText(bdg.Tier, bdg.Count)
	}

	embed := &discordgo.MessageEmbed{
		Title: "🏆 Трофеи · " + name,
		Color: trophyColor,
	}
	if badge != "" {
		embed.Description = "Лучшая медаль: " + badge
	}
	if b.season.ReigningChampion() == userID {
		embed.Description += "\n👑 Действующий чемпион"
	}
	if len(trophies) == 0 {
		embed.Description = "Медалей пока нет — их выдают, когда заканчивается сезон."
	}
	for _, t := range trophies {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("%s Сезон %d", season.BadgeText(t.Tier, 1), t.SeasonNumber),
			Value:  fmt.Sprintf("%d место из %d · %d", t.Place, t.Detail.Players, t.Points),
			Inline: true,
		})
	}

	// Five buttons to a row is Discord's limit; older seasons drop off rather
	// than break the message.
	var buttons []discordgo.MessageComponent
	for _, t := range trophies {
		if len(buttons) == 4 {
			break
		}
		buttons = append(buttons, discordgo.Button{
			Label:    fmt.Sprintf("Сезон %d", t.SeasonNumber),
			Style:    discordgo.SecondaryButton,
			CustomID: trophyData(t.SeasonNumber, userID),
			Emoji:    &discordgo.ComponentEmoji{Name: tierEmojiName(t.Tier)},
		})
	}
	buttons = append(buttons, b.launchAppButton())
	return embed, []discordgo.MessageComponent{discordgo.ActionsRow{Components: buttons}}, nil
}

// badgedName prefixes a player's name with their medal, for messages that name
// the player rather than merely answer them — a caught spawn, in front of the
// whole channel.
func (b *Bot) badgedName(userID int64, name string) string {
	badge, crown := b.season.BadgeOf(userID)
	return season.NameWithBadge(badge, crown, name)
}

// dropAuthor is the byline on a card someone just pulled: their best medal,
// their name, and the crown if they hold it. Only in a server — in a DM the
// player knows who rolled, and the line would repeat on every card.
func (b *Bot) dropAuthor(userID int64, name, guildID string) *discordgo.MessageEmbedAuthor {
	if guildID == "" {
		return nil
	}
	line := b.badgedName(userID, name)
	if line == name {
		return nil // nothing earned yet — no byline rather than a bare name
	}
	return &discordgo.MessageEmbedAuthor{Name: line}
}

// launchAppButton opens the embedded Activity, handled by the existing
// "launch_app" component route.
func (b *Bot) launchAppButton() discordgo.Button {
	return discordgo.Button{
		Label:    "Открыть в приложении",
		Style:    discordgo.PrimaryButton,
		CustomID: "launch_app",
		Emoji:    &discordgo.ComponentEmoji{Name: "🎴"},
	}
}

func (b *Bot) respondEmbedWithComponents(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent, update bool) {
	responseType := discordgo.InteractionResponseChannelMessageWithSource
	if update {
		responseType = discordgo.InteractionResponseUpdateMessage
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: responseType,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
		},
	}); err != nil {
		log.Printf("[DISCORD ERROR] сезонный ответ не отправлен: %v", err)
	}
}

func isComponent(i *discordgo.InteractionCreate) bool {
	return i.Type == discordgo.InteractionMessageComponent
}

// trophyData packs "trophy:<season>:<ownerID>" into a component id.
func trophyData(seasonNumber int, ownerID int64) string {
	return fmt.Sprintf("%s%d:%d", trophyPrefix, seasonNumber, ownerID)
}

func parseTrophyData(raw string) (seasonNumber int, ownerID int64) {
	parts := strings.Split(strings.TrimPrefix(raw, trophyPrefix), ":")
	seasonNumber, _ = strconv.Atoi(parts[0])
	if len(parts) > 1 {
		ownerID, _ = strconv.ParseInt(parts[1], 10, 64)
	}
	return seasonNumber, ownerID
}

// tierEmojiName is the bare emoji for a button, without the count: a button
// carries the season number already.
func tierEmojiName(tier string) string {
	return season.BadgeText(tier, 1)
}
