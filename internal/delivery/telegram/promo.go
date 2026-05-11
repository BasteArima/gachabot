package telegram

import (
	"strings"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) HandlePromo(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, _ := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	lang := getLang(dbUser, tgUser)

	args := ctx.Args()
	if len(args) == 0 {
		return ctx.Send(b.loc.Translate(lang, "promo_usage"), tele.ModeHTML)
	}

	code := args[0]
	reward, cards, err := b.service.RedeemPromo(dbUser.ID, code)
	if err != nil {
		var errKey string
		switch err.Error() {
		case "not_found":
			errKey = "promo_err_not_found"
		case "limit_reached":
			errKey = "promo_err_limit"
		case "already_used":
			errKey = "promo_err_used"
		case "expired":
			errKey = "promo_err_expired"
		default:
			errKey = "error_db"
		}
		return ctx.Send(b.loc.Translate(lang, errKey), tele.ModeHTML)
	}

	var sb strings.Builder
	sb.WriteString("<b>" + b.loc.Translate(lang, "promo_success_title") + "</b>\n\n")

	if reward.Points > 0 {
		sb.WriteString(b.loc.Translate(lang, "promo_reward_points", reward.Points) + "\n")
	}
	if reward.PremiumRolls > 0 {
		sb.WriteString(b.loc.Translate(lang, "promo_reward_rolls", reward.PremiumRolls) + "\n")
	}
	if len(cards) > 0 {
		sb.WriteString("\n" + b.loc.Translate(lang, "promo_reward_cards_count", len(cards)) + "\n")
		for _, c := range cards {
			sb.WriteString(b.loc.Translate(lang, "promo_reward_card", c.Name, c.PowerLevel) + "\n")
		}
	}

	text := sb.String()

	var sendErr error
	if len(cards) == 0 {
		sendErr = ctx.Send(text, tele.ModeHTML)
	} else if len(cards) == 1 {
		photo := &tele.Photo{File: tele.FromURL(cards[0].ImageURL), Caption: text}
		sendErr = ctx.Send(photo, tele.ModeHTML)
	} else {
		albumLimit := len(cards)
		if albumLimit > 10 {
			albumLimit = 10
		}
		var album tele.Album
		for i := 0; i < albumLimit; i++ {
			p := &tele.Photo{File: tele.FromURL(cards[i].ImageURL)}
			if i == 0 {
				p.Caption = text
			}
			album = append(album, p)
		}
		sendErr = ctx.SendAlbum(album, tele.ModeHTML)
	}

	// Sets from promo
	if len(reward.CompletedSets) > 0 {
		for _, set := range reward.CompletedSets {
			msg := b.loc.Translate(lang, "set_completed_msg", set.Name, set.Reward)
			_ = ctx.Reply(msg, tele.ModeHTML)
		}
	}

	return sendErr
}
