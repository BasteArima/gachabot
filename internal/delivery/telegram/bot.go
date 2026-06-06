package telegram

import (
	"context"
	"gachabot/internal/config"
	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service/duel"
	"gachabot/internal/service/gacha"
	"gachabot/internal/service/spawn"
	"gachabot/internal/service/suggest"
	"log"
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
	spawnService   *spawn.SpawnService
	adminID        int64
	adminChatID    int64
	backupChatID   int64
	require18Plus  bool
	groupLink      string
	startBannerURL string
}

func NewBot(repo *repository.PostgresRepo, rdb *redis.Client, gs *gacha.GachaService, ds *duel.DuelService, ss *suggest.SuggestService, sp *spawn.SpawnService, loc *i18n.Localizer, cfg config.TelegramConfig) (*Bot, error) {
	pref := tele.Settings{
		Token:  cfg.Token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
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
		spawnService:   sp,
		adminID:        cfg.AdminID,
		// Telegram supergroup IDs are negative; env stores the positive part.
		adminChatID:    -cfg.SuggestsGroupID,
		backupChatID:   -cfg.BackupGroupID,
		require18Plus:  cfg.Require18Plus,
		groupLink:      cfg.GroupLink,
		startBannerURL: cfg.StartBannerURL,
	}

	tgBot.setupRoutes()
	return tgBot, nil
}

func (b *Bot) Start() {
	log.Println("[TELEGRAM] The bot is running and listening for updates!")
	b.bot.Start()
}

func (b *Bot) setupRoutes() {
	b.bot.Use(b.AdminStateMiddleware)

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
	b.bot.Handle(tele.OnDocument, b.HandleDocument)

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

	// Chat registry (spawns/broadcasts): track add/remove of the bot in groups
	b.bot.Handle(tele.OnMyChatMember, b.HandleMyChatMember)

	// Chat spawns
	b.bot.Handle("/claim", b.HandleClaimCommand)
	b.bot.Handle("\fspawn_claim", b.HandleSpawnClaim)

	// Chat spawns: admin config & testing
	b.bot.Handle("/spawnnow", b.HandleSpawnNow)
	b.bot.Handle("/spawnplan", b.HandleSpawnPlan)
	b.bot.Handle("/spawn_export", b.HandleSpawnExport)
	b.bot.Handle("/spawn_import", b.HandleSpawnImport)
	b.bot.Handle("\fspawn_apply", b.HandleSpawnApply)
	b.bot.Handle("\fspawn_cancel", b.HandleSpawnCancel)

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

	// Global message
	b.bot.Handle("/globalmsg", b.HandleGlobalMsg)
	b.bot.Handle("\fglobal_cancel", b.HandleGlobalCancel)
	b.bot.Handle("\fglobal_send", b.HandleGlobalSend)

	// Admin: content themes & help
	b.bot.Handle("/admin", b.HandleAdminHelp)
	b.bot.Handle("/theme_export", b.HandleThemeExport)
	b.bot.Handle("/theme_import", b.HandleThemeImport)
	b.bot.Handle("\ftheme_apply", b.HandleThemeApply)
	b.bot.Handle("\ftheme_cancel", b.HandleThemeCancel)
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
