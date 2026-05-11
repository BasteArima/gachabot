package telegram

import (
	"context"
	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service/duel"
	"gachabot/internal/service/gacha"
	"gachabot/internal/service/suggest"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	tele "gopkg.in/telebot.v3"
)

type Bot struct {
	bot            *tele.Bot
	repo           *repository.PostgresRepo
	service        *gacha.GachaService
	duelService    *duel.DuelService
	loc            *i18n.Localizer
	rdb            *redis.Client
	suggestService *suggest.SuggestService
	adminID        int64
	adminChatID    int64
	backupChatID   int64
	require18Plus  bool
	groupLink      string
}

func NewBot(token string, repo *repository.PostgresRepo, rdb *redis.Client, gs *gacha.GachaService, ds *duel.DuelService, ss *suggest.SuggestService, loc *i18n.Localizer, adminID int64) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	rawAdminChatID := os.Getenv("ADMIN_TG_SUGGESTS_GROUP_ID")
	adminChatIDParsed, err := strconv.ParseInt(rawAdminChatID, 10, 64)
	if err != nil {
		log.Printf("Warning: ADMIN_TG_SUGGESTS_GROUP_ID is not a valid number: %v. Setting to 0.", err)
		adminChatIDParsed = 0
	}

	rawBackupChatID := os.Getenv("ADMIN_TG_BACKUP_GROUP_ID")
	backupID, err := strconv.ParseInt(rawBackupChatID, 10, 64)
	if err != nil {
		log.Printf("Warning: ADMIN_TG_BACKUP_GROUP_ID is not a valid number: %v. Setting to 0.", err)
		backupID = 0
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	tgBot := &Bot{
		bot:            b,
		repo:           repo,
		service:        gs,
		duelService:    ds,
		loc:            loc,
		rdb:            rdb,
		suggestService: ss,
		adminID:        adminID,
		adminChatID:    -adminChatIDParsed,
		backupChatID:   -backupID,
		require18Plus:  os.Getenv("REQUIRE_18_PLUS_CONFIRM") == "true",
		groupLink:      os.Getenv("BOT_GROUP_LINK"),
	}

	tgBot.setupRoutes()
	return tgBot, nil
}

func (b *Bot) Start() {
	log.Println("[TELEGRAM] The bot is running and listening for updates!")
	b.bot.Start()
}

func (b *Bot) setupRoutes() {
	b.bot.Handle("/start", b.HandleStart)
	b.bot.Handle("/roll", b.HandleRoll)
	b.bot.Handle("/profile", b.HandleProfile)
	b.bot.Handle("\fcards_nav", b.HandleCardsNav)
	b.bot.Handle("\fback_profile", b.HandleBackToProfile)

	// TOP
	//b.bot.Handle("/top", b.HandleLocalTop) // local chat top
	b.bot.Handle("/top", b.HandleGlobalTop)
	b.bot.Handle("\ftop_btn", b.HandleTopCallback)

	// Help & locale
	b.bot.Handle("/help", b.HandleHelp)
	b.bot.Handle("\fhelp_nav", b.HandleHelpCallback)
	b.bot.Handle("/locale", b.HandleLocale)
	b.bot.Handle("\flang_set", b.HandleLanguageSetCallback)

	// Duels
	b.bot.Handle("/duel", b.HandleDuel)
	b.bot.Handle("\fduel_accept", b.HandleDuelCallback)
	b.bot.Handle("\fduel_cancel", b.HandleDuelCallback)

	// Craft & shop
	b.bot.Handle("/craft", b.HandleCraft)
	b.bot.Handle("/shop", b.HandleDonate)
	b.bot.Handle("\fshop_rolls_menu", b.HandleShopMenu)
	b.bot.Handle("\fbuy_invoice", b.HandleSendInvoice)
	b.bot.Handle("\froll_again", b.HandleRollAgainCallback)
	b.bot.Handle(tele.OnCheckout, b.HandlePreCheckout)
	b.bot.Handle(tele.OnPayment, b.HandlePayment)

	// Promo
	b.bot.Handle("/promo", b.HandlePromo)
	b.bot.Handle("/addpromo", b.HandleAddPromo)
	b.bot.Handle("/createpromo", b.HandleCreatePromo)

	// Sets
	b.bot.Handle("\fsets_nav", b.HandleSetsList)
	b.bot.Handle("\fset_view", b.HandleSetView)
	b.bot.Handle("\fset_equip", b.HandleEquipAura)

	// Suggest
	b.bot.Handle("\fsuggest_start", b.HandleSuggestStart)
	b.bot.Handle(tele.OnPhoto, b.HandleMediaSuggest)
	b.bot.Handle(tele.OnDocument, b.HandleMediaSuggest)

	// Suggest quiz
	b.bot.Handle("\fs_q1_yes", b.HandleSuggestQuiz)
	b.bot.Handle("\fs_q1_no", b.HandleSuggestQuiz)
	b.bot.Handle("\fs_q2_yes", b.HandleSuggestQuiz)
	b.bot.Handle("\fs_q2_no", b.HandleSuggestQuiz)
	b.bot.Handle("\fs_q3_43", b.HandleSuggestQuiz)
	b.bot.Handle("\fs_q3_34", b.HandleSuggestQuiz)
	b.bot.Handle("\fs_q3_11", b.HandleSuggestQuiz)

	// Suggest close
	b.bot.Handle("\fs_cancel", b.HandleSuggestCancel)
	b.bot.Handle("\fs_close", b.HandleSuggestClose)

	// Other
	b.bot.Handle("/link", b.HandleLinkStart)
	b.bot.Handle(tele.OnText, b.HandleTextFallback)
	b.bot.Handle("\fadult_yes", b.HandleAdultConfirm)
	b.bot.Handle("\fadult_no", b.HandleAdultReject)

	// Short buttons
	b.bot.Handle(&tele.Btn{Unique: "roll_shortcut"}, b.HandleRollShortcut)
	b.bot.Handle(&tele.Btn{Unique: "profile_menu"}, b.HandleProfileMenu)
	b.bot.Handle(&tele.Btn{Unique: "start_menu"}, b.HandleStartMenu)
	b.bot.Handle(&tele.Btn{Unique: "help_menu"}, b.HandleHelpMenu)
}

// Implementing the LinkProvider interface for a Discord bot
func (b *Bot) GetIDByCode(code string) (int64, bool) {
	ctx := context.Background()

	// Get the ID by code
	val, err := b.rdb.Get(ctx, "link_code:"+code).Int64()
	if err == redis.Nil {
		return 0, false // Кода нет или он протух
	} else if err != nil {
		log.Println("Redis get error:", err)
		return 0, false
	}

	b.rdb.Del(ctx, "link_code:"+code)

	return val, true
}

// NotifyAdmin sends a message to the Telegram admin chat (used for the Discord bot)
func (b *Bot) NotifyAdmin(text string, imageURL string) {
	adminChat := &tele.Chat{ID: b.adminChatID}
	photo := &tele.Photo{
		File:    tele.FromURL(imageURL),
		Caption: text,
	}
	_, err := b.bot.Send(adminChat, photo, tele.ModeHTML)
	if err != nil {
		log.Printf("Ошибка пересылки предложки: %v", err)
	}
}
