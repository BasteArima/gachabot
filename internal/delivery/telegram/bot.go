package telegram

import (
	"context"
	"gachabot/internal/i18n"
	"gachabot/internal/repository"
	"gachabot/internal/service"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	tele "gopkg.in/telebot.v3"
)

// Обертка над телеграм-ботом
type Bot struct {
	bot         *tele.Bot
	repo        *repository.PostgresRepo // В идеале тут должны быть интерфейсы, но пока оставим так
	service     *service.GachaService
	duelService *service.DuelService
	loc         *i18n.Localizer
	rdb         *redis.Client
	adminChatID int64
}

// Конструктор бота
func NewBot(token string, repo *repository.PostgresRepo, rdb *redis.Client, gs *service.GachaService, ds *service.DuelService, loc *i18n.Localizer) (*Bot, error) {
	pref := tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	}

	b, err := tele.NewBot(pref)
	if err != nil {
		return nil, err
	}

	tgBot := &Bot{
		bot:         b,
		repo:        repo,
		service:     gs,
		duelService: ds,
		loc:         loc,
		rdb:         rdb,
		adminChatID: -5214417967,
	}

	// Регистрируем роуты прямо при создании
	tgBot.setupRoutes()

	return tgBot, nil
}

// Запуск поллинга
func (b *Bot) Start() {
	log.Println("[TELEGRAM] Бот запущен и слушает обновления!")
	b.bot.Start()
}

// setupRoutes содержит все команды (бывший router.go)
func (b *Bot) setupRoutes() {
	b.bot.Handle("/start", b.HandleStart)
	b.bot.Handle("/roll", b.HandleRoll)
	b.bot.Handle("/profile", b.HandleProfile)
	b.bot.Handle("\fcards_nav", b.HandleCardsNav)
	b.bot.Handle("\fback_profile", b.HandleBackToProfile)

	// Топы
	b.bot.Handle("/top", b.HandleLocalTop)
	b.bot.Handle("/globaltop", b.HandleGlobalTop)
	b.bot.Handle("\ftop_btn", b.HandleTopCallback)

	// Помощь и язык
	b.bot.Handle("/help", b.HandleHelp)
	b.bot.Handle("\fhelp_nav", b.HandleHelpCallback)
	b.bot.Handle("/locale", b.HandleLocale)
	b.bot.Handle("\flang_set", b.HandleLanguageSetCallback)

	// Дуэли
	b.bot.Handle("/duel", b.HandleDuel)
	b.bot.Handle("\fduel_accept", b.HandleDuelCallback)
	b.bot.Handle("\fduel_cancel", b.HandleDuelCallback)

	// Крафт и шоп
	b.bot.Handle("/craft", b.HandleCraft)
	b.bot.Handle("/shop", b.HandleDonate)
	b.bot.Handle("\fshop_rolls_menu", b.HandleShopMenu)
	b.bot.Handle("\fbuy_invoice", b.HandleSendInvoice)
	b.bot.Handle("\froll_again", b.HandleRollAgainCallback)
	b.bot.Handle(tele.OnCheckout, b.HandlePreCheckout)
	b.bot.Handle(tele.OnPayment, b.HandlePayment)

	// Предложка
	b.bot.Handle("\fsuggest_start", b.HandleSuggestStart)
	// ... (остальные кнопки предложки)
	b.bot.Handle(tele.OnPhoto, b.HandleMediaSuggest)
	b.bot.Handle(tele.OnDocument, b.HandleMediaSuggest)

	// Промокоды
	b.bot.Handle("/promo", b.HandlePromo)
	b.bot.Handle("/addpromo", b.HandleAddPromo)
	b.bot.Handle("/createpromo", b.HandleCreatePromo)

	// Разное
	b.bot.Handle("/link", b.HandleLinkStart)
	b.bot.Handle(tele.OnText, b.HandleTextFallback)
}

// Реализация интерфейса LinkProvider для Дискорда
func (b *Bot) GetIDByCode(code string) (int64, bool) {
	ctx := context.Background()

	// Получаем ID по коду
	val, err := b.rdb.Get(ctx, "link_code:"+code).Int64()
	if err == redis.Nil {
		return 0, false // Кода нет или он протух
	} else if err != nil {
		log.Println("Redis get error:", err)
		return 0, false
	}

	// Если код нашли, сразу удаляем его (он одноразовый)
	b.rdb.Del(ctx, "link_code:"+code)

	return val, true
}

// NotifyAdmin отправляет сообщение в админский чат Телеграма (используется Дискордом)
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
