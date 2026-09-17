package telegram

import (
	"log"
	"strconv"
	"strings"

	"gachabot/internal/service/season"

	tele "gopkg.in/telebot.v3"
)

// Seasons in chat. Most players live here rather than in the app, so the
// standings and the trophy shelf have to open without leaving Telegram.
//
// The shelf and an opened medal are two views of one message: pressing a season
// edits the message in place and the back button edits it back, the way the Art
// Guess board already works. A new message per tap would bury the chat.
//
// Texts are sent without a parse mode: they are built from player names, and a
// nickname containing "<" would otherwise break the message.

// Callback data is "<season>|<fromProfile>|<ownerID>". The owner matters in
// groups: the message is shared, so without it anyone pressing a button would
// redraw someone else's shelf over the one on screen.
const trophyCallbackSep = "|"

// HandleSeason renders the current standings for whoever asked.
func (b *Bot) HandleSeason(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return ctx.Send("Техническая ошибка БД.")
	}
	b.service.TrackChat(dbUser.ID, ctx.Chat().ID)

	cur := b.season.Current()
	if cur == nil {
		return ctx.Send("Сезон сейчас не идёт. Когда он начнётся, здесь появится счёт.")
	}

	rows, _, err := b.season.Scoreboard()
	if err != nil {
		log.Printf("[SEASON] не удалось прочитать счёт: %v", err)
		return ctx.Send("Не получилось прочитать счёт сезона, попробуй позже.")
	}
	b.season.Decorate(rows)
	elapsed, _ := b.season.Days()

	text := season.BoardText(cur.Number, cur.Title, elapsed, b.season.DaysLeft(), rows, dbUser.ID)

	menu := &tele.ReplyMarkup{}
	row := []tele.Btn{menu.Data("🏆 Мои трофеи", "trophy", trophyData(0, false, dbUser.ID))}
	if b.bot.Me != nil && b.bot.Me.Username != "" {
		row = append(row, menu.URL("Открыть в приложении", b.appURL("season")))
	}
	menu.Inline(menu.Row(row...))
	return ctx.Send(text, menu)
}

// HandleTrophies renders the caller's shelf as a message of its own.
func (b *Bot) HandleTrophies(ctx tele.Context) error {
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return ctx.Send("Техническая ошибка БД.")
	}

	text, markup, err := b.shelfView(dbUser.ID, displayName(tgUser), false)
	if err != nil {
		return ctx.Send("Не получилось прочитать трофеи, попробуй позже.")
	}
	return ctx.Send(text, markup)
}

// HandleTrophyCallback switches the message between the shelf and one medal.
// Data is "<seasonNumber>|<fromProfile>", where season 0 means the shelf.
func (b *Bot) HandleTrophyCallback(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return nil
	}

	parts := strings.Split(ctx.Callback().Data, trophyCallbackSep)
	number, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	fromProfile := len(parts) > 1 && strings.TrimSpace(parts[1]) == "1"
	owner := int64(0)
	if len(parts) > 2 {
		owner, _ = strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
	}

	// In a group the board is one shared message. Redrawing it as the presser's
	// own shelf would wipe what everyone else is looking at.
	if owner != 0 && owner != dbUser.ID {
		return ctx.Respond(&tele.CallbackResponse{
			Text:      "Это чужие трофеи. Свои открой командой /trophies",
			ShowAlert: true,
		})
	}

	if number == 0 {
		text, markup, err := b.shelfView(dbUser.ID, displayName(tgUser), fromProfile)
		if err != nil {
			return nil
		}
		return b.editView(ctx, text, markup)
	}

	trophies, _, err := b.season.Trophies(dbUser.ID)
	if err != nil {
		return nil
	}
	for _, t := range trophies {
		if t.SeasonNumber != number {
			continue
		}
		menu := &tele.ReplyMarkup{}
		menu.Inline(menu.Row(menu.Data("‹ Все трофеи", "trophy", trophyData(0, fromProfile, dbUser.ID))))
		return b.editView(ctx, season.TrophyText(t), menu)
	}
	return nil
}

// HandleProfileTrophies opens the shelf inside the profile message, so the back
// button lands where the player came from.
func (b *Bot) HandleProfileTrophies(ctx tele.Context) error {
	_ = ctx.Respond()
	tgUser := ctx.Sender()
	dbUser, err := b.repo.GetOrCreateUserByTelegramID(tgUser.ID, tgUser.Username, tgUser.FirstName, tgUser.LastName)
	if err != nil {
		return nil
	}
	text, markup, err := b.shelfView(dbUser.ID, displayName(tgUser), true)
	if err != nil {
		return nil
	}
	return b.editView(ctx, text, markup)
}

// shelfView builds the shelf text and its buttons: one per medal, plus a way
// back to the profile when that is where this started.
func (b *Bot) shelfView(userID int64, name string, fromProfile bool) (string, *tele.ReplyMarkup, error) {
	trophies, _, err := b.season.Trophies(userID)
	if err != nil {
		log.Printf("[SEASON] не удалось прочитать трофеи игрока %d: %v", userID, err)
		return "", nil, err
	}
	badges, err := b.season.Badges([]int64{userID})
	if err != nil {
		log.Printf("[SEASON] не удалось прочитать значок игрока %d: %v", userID, err)
	}
	badge := ""
	if bdg, ok := badges[userID]; ok {
		badge = season.BadgeText(bdg.Tier, bdg.Count)
	}

	text := season.ShelfText(name, trophies, badge, b.season.ReigningChampion() == userID)

	menu := &tele.ReplyMarkup{}
	var rows []tele.Row
	var line []tele.Btn
	for _, t := range trophies {
		line = append(line, menu.Data(
			season.BadgeText(t.Tier, 1)+" Сезон "+strconv.Itoa(t.SeasonNumber),
			"trophy", trophyData(t.SeasonNumber, fromProfile, userID)))
		// Three to a row: Telegram squeezes longer rows into unreadable slivers.
		if len(line) == 3 {
			rows = append(rows, menu.Row(line...))
			line = nil
		}
	}
	if len(line) > 0 {
		rows = append(rows, menu.Row(line...))
	}
	if b.bot.Me != nil && b.bot.Me.Username != "" {
		rows = append(rows, menu.Row(menu.URL("Открыть в приложении", b.appURL("trophies"))))
	}
	if fromProfile {
		rows = append(rows, menu.Row(menu.Data("‹ Назад в профиль", "back_profile")))
	}
	menu.Inline(rows...)
	return text, menu, nil
}

// editView edits whatever the message actually is. A profile with an avatar is a
// photo, and a photo's text is its caption — editMessageText would fail on it.
func (b *Bot) editView(ctx tele.Context, text string, markup *tele.ReplyMarkup) error {
	msg := ctx.Message()
	if msg != nil && msg.Photo != nil {
		_, err := b.bot.EditCaption(msg, text, &tele.SendOptions{ReplyMarkup: markup})
		if err != nil && !strings.Contains(err.Error(), "message is not modified") {
			return err
		}
		return nil
	}
	err := ctx.Edit(text, markup)
	if err != nil && !strings.Contains(err.Error(), "message is not modified") {
		return err
	}
	return nil
}

// appURL opens the Mini App on a given screen — the deep link the app reads on
// start, so "Открыть в приложении" lands on the trophies rather than the hub.
func (b *Bot) appURL(screen string) string {
	return "https://t.me/" + b.bot.Me.Username + "?startapp=" + screen
}

// trophyData packs a callback: which medal, whether the profile is behind it,
// and whose shelf this is.
func trophyData(seasonNumber int, fromProfile bool, ownerID int64) string {
	flag := "0"
	if fromProfile {
		flag = "1"
	}
	return strconv.Itoa(seasonNumber) + trophyCallbackSep + flag + trophyCallbackSep + strconv.FormatInt(ownerID, 10)
}

func displayName(u *tele.User) string {
	if u == nil {
		return "Игрок"
	}
	if name := strings.TrimSpace(u.FirstName + " " + u.LastName); name != "" {
		return name
	}
	if u.Username != "" {
		return u.Username
	}
	return "Игрок"
}
