package discord

import (
	"log"
	"strconv"

	"gachabot/internal/i18n"
	"gachabot/internal/models"
	"gachabot/internal/service/spawn"

	"github.com/bwmarrin/discordgo"
)

const spawnLang = "ru"
const spawnColor = 0xf1c40f

// SendSpawn implements spawn.Spawner for Discord.
func (b *Bot) SendSpawn(chat models.Chat, view spawn.SpawnView) (int64, error) {
	channelID := strconv.FormatInt(chat.ChatID, 10)
	embed := &discordgo.MessageEmbed{
		Title: b.loc.Translate(spawnLang, "spawn_title"),
		Description: b.loc.Translate(spawnLang, "spawn_appeared", i18n.Args{
			"name":    view.Card.Name,
			"rarity":  b.loc.Rarity(spawnLang, view.RarityName),
			"seconds": view.WindowSeconds,
		}),
		Image: &discordgo.MessageEmbedImage{URL: view.Card.ImageURL},
		Color: spawnColor,
	}
	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				CustomID: "spawn_claim:" + strconv.FormatInt(view.SpawnID, 10),
				Label:    b.loc.Translate(spawnLang, "btn_spawn_claim"),
				Style:    discordgo.SuccessButton,
			},
		}},
	}
	msg, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Embeds:     []*discordgo.MessageEmbed{embed},
		Components: components,
	})
	if err != nil {
		return 0, err
	}
	return parseID(msg.ID), nil
}

// EditSpawnExpired implements spawn.Spawner for Discord.
func (b *Bot) EditSpawnExpired(meta spawn.SpawnMeta, card *models.Card) {
	embed := &discordgo.MessageEmbed{
		Description: b.loc.Translate(spawnLang, "spawn_expired", i18n.Args{"name": card.Name}),
		Color:       0x95a5a6,
	}
	b.editSpawnMessage(meta, embed)
}

// handleSpawnClaimComponent handles the claim button press.
func (b *Bot) handleSpawnClaimComponent(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, lang string, spawnID int64) {
	res, err := b.spawnService.Claim(spawn.PlatformDiscord, dbUser.ID, spawnID)
	if err != nil {
		log.Printf("[DS spawn] claim error: %v", err)
		b.respondEphemeral(s, i, b.loc.Translate(lang, "error_tech"))
		return
	}

	switch res.Outcome {
	case spawn.OutcomeWon:
		embed := b.spawnClaimedEmbed(discordDisplayName(i), res)
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Embeds:     []*discordgo.MessageEmbed{embed},
				Components: []discordgo.MessageComponent{},
			},
		})
	case spawn.OutcomeTaken:
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_toast_taken"))
	case spawn.OutcomeAlreadyWave:
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_toast_already"))
	default:
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_toast_expired"))
	}
}

// handleClaimCommand handles /claim — claims the active spawn in this channel.
func (b *Bot) handleClaimCommand(s *discordgo.Session, i *discordgo.InteractionCreate, dbUser *models.User, lang string) {
	if i.GuildID == "" {
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_group_only"))
		return
	}
	channelID := parseID(i.ChannelID)
	spawnID, ok := b.spawnService.ActiveSpawnInChat(spawn.PlatformDiscord, channelID)
	if !ok {
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_no_active"))
		return
	}

	res, err := b.spawnService.Claim(spawn.PlatformDiscord, dbUser.ID, spawnID)
	if err != nil {
		log.Printf("[DS spawn] /claim error: %v", err)
		b.respondEphemeral(s, i, b.loc.Translate(lang, "error_tech"))
		return
	}

	switch res.Outcome {
	case spawn.OutcomeWon:
		// Edit the original spawn message, confirm to the claimer privately.
		if res.Meta != nil {
			b.editSpawnMessage(*res.Meta, b.spawnClaimedEmbed(discordDisplayName(i), res))
		}
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_toast_won", i18n.Args{"coins": res.Reward.Coins}))
	case spawn.OutcomeTaken:
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_toast_taken"))
	case spawn.OutcomeAlreadyWave:
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_toast_already"))
	default:
		b.respondEphemeral(s, i, b.loc.Translate(lang, "spawn_no_active"))
	}
}

func (b *Bot) spawnClaimedEmbed(winner string, res spawn.ClaimResult) *discordgo.MessageEmbed {
	r := res.Reward
	args := i18n.Args{
		"winner":    winner,
		"name":      res.Card.Name,
		"rarity":    b.loc.Rarity(spawnLang, res.RarityName),
		"coins":     r.Coins,
		"fragments": r.FragmentsCount,
	}
	var key string
	switch {
	case r.CardAssembled:
		key = "spawn_claimed_assembled"
	case r.IsFragment:
		key = "spawn_claimed_fragment"
	case r.IsDuplicate:
		key = "spawn_claimed_duplicate"
	default:
		key = "spawn_claimed"
	}
	return &discordgo.MessageEmbed{
		Description: b.loc.Translate(spawnLang, key, args),
		Image:       &discordgo.MessageEmbedImage{URL: res.Card.ImageURL},
		Color:       0x2ecc71,
	}
}

func (b *Bot) editSpawnMessage(meta spawn.SpawnMeta, embed *discordgo.MessageEmbed) {
	empty := []discordgo.MessageComponent{}
	_, err := b.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    strconv.FormatInt(meta.ChatID, 10),
		ID:         strconv.FormatInt(meta.MessageID, 10),
		Embeds:     &[]*discordgo.MessageEmbed{embed},
		Components: &empty,
	})
	if err != nil {
		log.Printf("[DS spawn] edit message failed: %v", err)
	}
}

func discordDisplayName(i *discordgo.InteractionCreate) string {
	if i.Member != nil {
		if i.Member.Nick != "" {
			return i.Member.Nick
		}
		if i.Member.User != nil {
			if i.Member.User.GlobalName != "" {
				return i.Member.User.GlobalName
			}
			return i.Member.User.Username
		}
	}
	if i.User != nil {
		if i.User.GlobalName != "" {
			return i.User.GlobalName
		}
		return i.User.Username
	}
	return "Player"
}
