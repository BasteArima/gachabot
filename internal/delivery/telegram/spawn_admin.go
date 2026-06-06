package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"gachabot/internal/service/spawn"

	tele "gopkg.in/telebot.v3"
)

func (b *Bot) spawnImportKey() string {
	return fmt.Sprintf("spawn_import:%d", b.adminID)
}

// HandleSpawnNow forces an immediate spawn in the current chat. Admin only,
// groups only — handy for testing in a dedicated test chat without waiting for
// the schedule or spamming other chats.
func (b *Bot) HandleSpawnNow(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}
	if !isGroup(ctx.Chat()) {
		return ctx.Send("⚠️ Запусти /spawnnow в групповом чате — спавн появится здесь.")
	}
	name, err := b.spawnService.SpawnNow(spawn.PlatformTelegram, ctx.Chat().ID)
	if err != nil {
		return ctx.Send("❌ Не удалось: " + err.Error())
	}
	return ctx.Send(fmt.Sprintf("✅ Заспавнил «%s» в этом чате. Лови!", name))
}

// HandleSpawnExport sends the current spawn config as spawn_config.json. Admin only.
func (b *Bot) HandleSpawnExport(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}
	data, err := b.spawnService.CurrentConfigJSON()
	if err != nil {
		return ctx.Send("❌ Ошибка экспорта: " + err.Error())
	}
	doc := &tele.Document{
		File:     tele.FromReader(bytes.NewReader(data)),
		FileName: "spawn_config.json",
		Caption:  "⚙️ Текущий конфиг спавнов. Отредактируй и пришли обратно с подписью /spawn_import.",
	}
	return ctx.Send(doc)
}

// HandleSpawnImport validates an attached spawn_config.json and shows a dry-run
// preview with a confirmation button. Nothing is saved until /spawn_apply. Admin only.
func (b *Bot) HandleSpawnImport(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}
	doc := ctx.Message().Document
	if doc == nil {
		return ctx.Send("Прикрепи файл spawn_config.json, а в подписи укажи /spawn_import.")
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

	cfg, err := spawn.ParseConfig(data)
	if err != nil {
		return ctx.Send("❌ Невалидный конфиг: " + err.Error())
	}

	if err := b.rdb.Set(context.Background(), b.spawnImportKey(), string(data), 10*time.Minute).Err(); err != nil {
		return ctx.Send("❌ Не удалось сохранить во временное хранилище: " + err.Error())
	}

	var sb strings.Builder
	sb.WriteString("📋 <b>Предпросмотр конфига спавнов</b>\n\n")
	sb.WriteString(spawn.Summarize(cfg))
	sb.WriteString("\n<i>Это полностью заменит текущий конфиг. Применить?</i>")

	btnYes := tele.InlineButton{Unique: "spawn_apply", Text: "✅ Применить"}
	btnNo := tele.InlineButton{Unique: "spawn_cancel", Text: "❌ Отмена"}
	markup := &tele.ReplyMarkup{InlineKeyboard: [][]tele.InlineButton{{btnYes, btnNo}}}
	return ctx.Send(sb.String(), tele.ModeHTML, markup)
}

// HandleSpawnApply persists the previously previewed config. Admin only.
func (b *Bot) HandleSpawnApply(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}
	data, err := b.rdb.Get(context.Background(), b.spawnImportKey()).Result()
	if err != nil {
		_ = ctx.Respond(&tele.CallbackResponse{Text: "Срок действия истёк — пришлите файл заново.", ShowAlert: true})
		return ctx.Edit("⌛ Импорт устарел. Пришлите spawn_config.json заново.")
	}

	if _, err := b.spawnService.SaveConfigJSON([]byte(data)); err != nil {
		_ = ctx.Respond()
		return ctx.Edit("❌ Не удалось применить: " + err.Error())
	}
	b.rdb.Del(context.Background(), b.spawnImportKey())

	_ = ctx.Respond(&tele.CallbackResponse{Text: "Применено"})
	return ctx.Edit("✅ Конфиг спавнов обновлён.")
}

// HandleSpawnCancel discards a pending spawn config import. Admin only.
func (b *Bot) HandleSpawnCancel(ctx tele.Context) error {
	if ctx.Sender().ID != b.adminID {
		return nil
	}
	b.rdb.Del(context.Background(), b.spawnImportKey())
	_ = ctx.Respond(&tele.CallbackResponse{Text: "Отменено"})
	return ctx.Edit("❌ Импорт конфига отменён.")
}
