package backup

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

type Sender interface {
	SendBackup(filePath string) error
}

type BackupService struct {
	hour   int
	minute int
	dir    string
	sender Sender
}

func NewBackupService(hour, minute int, dir string, sender Sender) *BackupService {
	return &BackupService{
		hour:   hour,
		minute: minute,
		dir:    dir,
		sender: sender,
	}
}

func (s *BackupService) Start() {
	go func() {
		loc, err := time.LoadLocation("Europe/Moscow")
		if err != nil {
			loc = time.FixedZone("MSK", 3*60*60)
		}

		for {
			now := time.Now().In(loc)
			next := time.Date(now.Year(), now.Month(), now.Day(), s.hour, s.minute, 0, 0, loc)

			if now.After(next) {
				next = next.Add(24 * time.Hour)
			}

			sleepDuration := time.Until(next)
			log.Printf("📦 [BACKUP SERVICE] Next backup scheduled for %v (in %v)", next.Format("15:04:05"), sleepDuration.Round(time.Second))

			time.Sleep(sleepDuration)
			s.processLatestBackup()
		}
	}()
}

func (s *BackupService) processLatestBackup() {
	var newestFile string
	var newestTime time.Time

	files, err := os.ReadDir(s.dir)
	if err != nil {
		log.Printf("❌ [BACKUP SERVICE] Error reading backup directory: %v", err)
		return
	}

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
			newestFile = filepath.Join(s.dir, f.Name())
		}
	}

	if newestFile == "" {
		log.Println("⚠️ [BACKUP SERVICE] Backup file not found in directory!")
		return
	}

	err = s.sender.SendBackup(newestFile)
	if err != nil {
		log.Printf("❌ [BACKUP SERVICE] Error sending backup via sender: %v", err)
	} else {
		log.Printf("✅ [BACKUP SERVICE] Backup sent successfully: %s", newestFile)
	}
}
