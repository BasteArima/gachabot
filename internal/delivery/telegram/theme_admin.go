package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"gachabot/internal/theme"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) themeImportKey() string {
	return fmt.Sprintf("theme_import:%d", b.adminID)
}

// HandleThemeExport dumps the current content (rarities/sets/cards) as a
// theme.json document. Admin only.
func (b *Bot) HandleThemeExport(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}

	t, err := theme.Export(b.repo.DB())
	if err != nil {
		return ctx.Send("❌ Ошибка экспорта: " + err.Error())
	}
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return ctx.Send("❌ Ошибка сериализации: " + err.Error())
	}

	doc := &tele.Document{
		File:     tele.FromReader(bytes.NewReader(data)),
		FileName: "theme.json",
		Caption:  fmt.Sprintf("📦 Тема: %d редкостей, %d сетов, %d карт", len(t.Rarities), len(t.Sets), len(t.Cards)),
	}
	return ctx.Send(doc)
}

// HandleThemeImport validates a theme.json attached to the message and shows a
// dry-run preview with a confirmation button. Nothing is written until the admin
// presses "Применить" (see HandleThemeApply). Admin only.
func (b *Bot) HandleThemeImport(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}

	doc := ctx.Message().Document
	if doc == nil {
		return ctx.Send("Прикрепи файл theme.json, а в подписи к нему укажи команду /theme_import.")
	}

	rc, err := ctx.Bot().File(&doc.File)
	if err != nil {
		return ctx.Send("❌ Не удалось скачать файл: " + err.Error())
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return ctx.Send("❌ Ошибка чтения файла: " + err.Error())
	}

	t, err := theme.Parse(data)
	if err != nil {
		return ctx.Send("❌ Неверный JSON: " + err.Error())
	}

	warns, err := t.Validate()
	if err != nil {
		return ctx.Send("❌ Невалидная тема: " + err.Error())
	}

	// Dry-run to compute the change preview without touching the database.
	rep, err := theme.Import(b.repo.DB(), t, theme.Options{DryRun: true})
	if err != nil {
		return ctx.Send("❌ Проверка не удалась: " + err.Error())
	}

	// Stash the validated file for the confirmation step.
	if err := b.rdb.Set(context.Background(), b.themeImportKey(), string(data), 10*time.Minute).Err(); err != nil {
		return ctx.Send("❌ Не удалось сохранить во временное хранилище: " + err.Error())
	}

	var sb strings.Builder
	sb.WriteString("📋 <b>Предпросмотр импорта</b>\n\n")
	sb.WriteString(fmt.Sprintf("Будет применено: <b>%d</b> редкостей, <b>%d</b> сетов, <b>%d</b> новых карт, <b>%d</b> обновлённых карт.\n",
		rep.RaritiesUpserted, rep.SetsUpserted, rep.CardsInserted, rep.CardsUpdated))
	for _, w := range warns {
		sb.WriteString("⚠ " + w + "\n")
	}
	sb.WriteString("\n<i>Существующее не удаляется, id карт сохраняются. Применить?</i>")

	btnYes := tele.InlineButton{Unique: "theme_apply", Text: "✅ Применить"}
	btnNo := tele.InlineButton{Unique: "theme_cancel", Text: "❌ Отмена"}
	markup := &tele.ReplyMarkup{InlineKeyboard: [][]tele.InlineButton{{btnYes, btnNo}}}
	return ctx.Send(sb.String(), tele.ModeHTML, markup)
}

// HandleThemeApply writes the previously previewed theme to the database. Admin only.
func (b *Bot) HandleThemeApply(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}

	data, err := b.rdb.Get(context.Background(), b.themeImportKey()).Result()
	if err != nil {
		_ = ctx.Respond(&tele.CallbackResponse{Text: "Срок действия истёк — загрузите файл заново.", ShowAlert: true})
		return ctx.Edit("⌛ Импорт устарел. Загрузите theme.json заново.")
	}

	t, err := theme.Parse([]byte(data))
	if err != nil {
		_ = ctx.Respond()
		return ctx.Edit("❌ Ошибка разбора сохранённого файла: " + err.Error())
	}

	rep, err := theme.Import(b.repo.DB(), t, theme.Options{})
	b.rdb.Del(context.Background(), b.themeImportKey())
	if err != nil {
		_ = ctx.Respond()
		return ctx.Edit("❌ Импорт не удался: " + err.Error())
	}

	_ = ctx.Respond(&tele.CallbackResponse{Text: "Применено"})
	return ctx.Edit("✅ " + rep.String())
}

// HandleThemeCancel discards a pending theme import. Admin only.
func (b *Bot) HandleThemeCancel(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}
	b.rdb.Del(context.Background(), b.themeImportKey())
	_ = ctx.Respond(&tele.CallbackResponse{Text: "Отменено"})
	return ctx.Edit("❌ Импорт отменён.")
}

// HandleDocument routes an incoming document: admin theme imports first, then the
// regular card-suggestion flow.
func (b *Bot) HandleDocument(ctx tele.Context) error {
	caption := strings.TrimSpace(ctx.Message().Caption)
	if ctx.Sender() != nil && ctx.Sender().ID == b.adminID {
		if strings.HasPrefix(caption, "/theme_import") {
			return b.HandleThemeImport(ctx)
		}
		if strings.HasPrefix(caption, "/spawn_import") {
			return b.HandleSpawnImport(ctx)
		}
	}
	return b.HandleMediaSuggest(ctx)
}

// HandleAdminHelp lists the available admin commands. Admin only (silent for others).
func (b *Bot) HandleAdminHelp(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}
	return ctx.Send(strings.Join([]string{
		"<b>🛠 Админ-команды</b>",
		"",
		"<b>Контент (темы):</b>",
		"/theme_export — выгрузить текущую тему в файл theme.json",
		"/theme_import — применить тему: прикрепи theme.json, в подписи укажи команду",
		"   • покажет предпросмотр и спросит подтверждение перед записью",
		"",
		"<b>Промокоды:</b>",
		"/addpromo КОД ОЧКИ КРУТКИ МАКС_ЮЗОВ — простой промокод",
		"/createpromo КОД ЛИМИТ ЧАСЫ {json} — промокод из JSON",
		"",
		"<b>Спавны карт:</b>",
		"/spawn_export — выгрузить текущий конфиг спавнов файлом",
		"/spawn_import — применить конфиг: прикрепи spawn_config.json с этой подписью",
		"/spawnnow — (в группе) сразу заспавнить карту в этом чате для теста",
		"/spawnplan — когда сегодня следующие автоспавны",
		"",
		"<b>Рассылка:</b>",
		"/globalmsg — одноразовое сообщение во все чаты",
		"",
		"/admin — этот список",
	}, "\n"), tele.ModeHTML)
}
