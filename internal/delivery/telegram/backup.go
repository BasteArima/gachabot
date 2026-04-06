package telegram

import (
	"log"
	"os"
	"path/filepath"
	"time"

	tele "gopkg.in/telebot.v3"
)

// StartBackupSender запускает вечный цикл для отправки бэкапов
func (b *Bot) StartBackupSender() {
	go func() {
		// Явно загружаем локацию Москвы
		loc, err := time.LoadLocation("Europe/Moscow")
		if err != nil {
			// Если вдруг в системе нет tzdata, используем фиксированную зону UTC+3
			loc = time.FixedZone("MSK", 3*60*60)
		}

		for {
			// Берем текущее время именно в московской локации
			now := time.Now().In(loc)

			// Планируем на 03:10 по Москве
			next := time.Date(now.Year(), now.Month(), now.Day(), 3, 10, 0, 0, loc)

			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}

			sleepDuration := time.Until(next)
			log.Printf("📦 [BACKUP SENDER] МСК время: %v. Отправка через %v", now.Format("15:04:05"), sleepDuration)
			time.Sleep(sleepDuration)

			b.sendLatestBackup()
		}
	}()
}

func (b *Bot) sendLatestBackup() {
	// Папка, куда падают ежедневные бэкапы (из контейнера pgbackups)
	dir := "/backups/daily"
	var newestFile string
	var newestTime time.Time

	files, err := os.ReadDir(dir)
	if err != nil {
		log.Printf("❌ [BACKUP SENDER] Ошибка чтения директории бэкапов: %v", err)
		return
	}

	// Ищем самый свежий файл в папке
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		info, err := f.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestTime) {
			newestTime = info.ModTime()
			newestFile = filepath.Join(dir, f.Name())
		}
	}

	if newestFile == "" {
		log.Println("⚠️ [BACKUP SENDER] Файл бэкапа не найден в папке!")
		return
	}

	doc := &tele.Document{
		File:    tele.FromDisk(newestFile),
		Caption: "📦 <b>Автоматический бэкап базы данных GachaBot</b>\n\nФайл можно использовать для восстановления через команду:\n<code>gunzip -c файл.sql.gz | docker exec -i gachabot_db psql -U postgres -d postgres</code>",
	}

	// Отправляем в твой секретный чат
	chat := &tele.Chat{ID: -5176167861}
	_, err = b.bot.Send(chat, doc, tele.ModeHTML)
	if err != nil {
		log.Printf("❌ [BACKUP SENDER] Ошибка отправки бэкапа в Telegram: %v", err)
	} else {
		log.Printf("✅ [BACKUP SENDER] Успешно отправлен бэкап: %s", newestFile)
	}
}
