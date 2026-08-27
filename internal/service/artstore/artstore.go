// Package artstore puts card art where the bot's image urls point. The art is
// served from a separate host (openresty over /var/www/api_files), so this is
// the only part of the system that writes files somewhere other than its own
// disk.
//
// Two drivers exist on purpose: sftp for production, and a plain directory for
// local development, where there is no remote host to talk to. The directory
// driver doubles as a fallback if SSH is ever unavailable.
package artstore

import (
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// ErrDisabled is returned when no driver is configured, so callers can tell
// "not set up" apart from "failed".
var ErrDisabled = errors.New("заливка арта не настроена")

// ErrExists guards an accidental overwrite: art is content, and clobbering a
// file that other cards may already reference is not recoverable from here.
var ErrExists = errors.New("файл уже существует")

const (
	// PlainDir and FramedDir mirror what gen.py writes and what cardart.Framed
	// swaps between in image urls.
	PlainDir  = "cards"
	FramedDir = "cards_framed"
)

// Folder names are the English raw_art folders (Rare, Mythical, …) and slugs are
// derived from card names. Both end up in a filesystem path, so both are matched
// against a tight charset rather than merely cleaned.
var (
	folderRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _-]{0,39}$`)
	slugRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,60}$`)
)

// driver is the storage behind the service: a remote host or a local directory.
type driver interface {
	// exists reports whether a file is already there.
	exists(rel string) (bool, error)
	// put writes the bytes, creating parent directories as needed.
	put(rel string, data []byte) error
	// describe names the driver for the admin UI and logs.
	describe() string
	// close releases a connection, if the driver holds one.
	close() error
}

// Service writes card art and reports the public urls it will be served under.
type Service struct {
	newDriver  func() (driver, error)
	publicBase string
	driverName string
}

// Config is read from the environment by FromEnv.
type Config struct {
	// PublicBase is the url the art host serves from, e.g. https://api.baste.ru.
	PublicBase string
	// LocalDir writes to a directory instead of a remote host (development).
	LocalDir string
	// SFTP settings; Host being set selects the sftp driver.
	SFTPHost     string
	SFTPUser     string
	SFTPPassword string
	SFTPRoot     string
	// SFTPHostKey pins the server's host key. Required: with password auth an
	// unverified host is a credential leak waiting to happen, so there is no
	// "trust anything" mode. Accepts either an ssh-keyscan line
	// ("ssh-ed25519 AAAA…") or a "SHA256:…" fingerprint.
	SFTPHostKey string
}

// New builds a service from config. It returns a service with no driver (every
// write answers ErrDisabled) rather than an error when nothing is configured —
// the panel then shows the tool as "not set up" instead of failing to start.
func New(cfg Config) *Service {
	s := &Service{publicBase: strings.TrimRight(cfg.PublicBase, "/")}

	switch {
	case cfg.SFTPHost != "":
		s.driverName = "sftp"
		s.newDriver = func() (driver, error) { return newSFTP(cfg) }
	case cfg.LocalDir != "":
		s.driverName = "dir"
		s.newDriver = func() (driver, error) { return newDir(cfg.LocalDir) }
	}
	return s
}

// Enabled reports whether art can be written at all.
func (s *Service) Enabled() bool { return s.newDriver != nil && s.publicBase != "" }

// DriverName is shown in the admin panel so it is obvious where files will land.
func (s *Service) DriverName() string {
	if s.newDriver == nil {
		return ""
	}
	return s.driverName
}

// PublicBase is the url prefix the stored files are served under.
func (s *Service) PublicBase() string { return s.publicBase }

// ValidFolder and ValidSlug are exported so handlers can reject bad input before
// any work starts, with a message naming the offending field.
func ValidFolder(folder string) bool { return folderRe.MatchString(folder) }
func ValidSlug(slug string) bool     { return slugRe.MatchString(slug) }

// Slugify turns a card name into a file name the way the art folders are named:
// lowercase, spaces to underscores, everything else dropped. "Bunny suit" →
// "bunny_suit". Returns "" when nothing usable is left.
func Slugify(name string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		case r == ' ' || r == '_' || r == '-':
			if b.Len() > 0 && !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

// rel builds the path of one file inside the art root.
func rel(dir, folder, slug, ext string) string {
	return path.Join(dir, folder, slug+ext)
}

// URLs reports where the pair will be served from once written.
func (s *Service) URLs(folder, slug, ext string) (plain, framed string) {
	return s.publicBase + "/" + rel(PlainDir, folder, slug, ext),
		s.publicBase + "/" + rel(FramedDir, folder, slug, ext)
}

// Existing reports which of the known extensions are already taken for a slug,
// so the panel can warn before anything is overwritten.
func (s *Service) Existing(folder, slug string) ([]string, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	if !ValidFolder(folder) || !ValidSlug(slug) {
		return nil, fmt.Errorf("плохая папка или имя файла")
	}

	d, err := s.newDriver()
	if err != nil {
		return nil, err
	}
	defer d.close()

	var found []string
	for _, ext := range []string{".webp", ".png"} {
		for _, dir := range []string{PlainDir, FramedDir} {
			ok, err := d.exists(rel(dir, folder, slug, ext))
			if err != nil {
				return nil, err
			}
			if ok {
				found = append(found, rel(dir, folder, slug, ext))
			}
		}
	}
	return found, nil
}

// Put writes the frameless and framed files for one card. Both are written or
// neither is attempted: the pair is checked for existing files first, unless
// overwrite is set.
func (s *Service) Put(folder, slug, ext string, plain, framed []byte, overwrite bool) (plainURL, framedURL string, err error) {
	if !s.Enabled() {
		return "", "", ErrDisabled
	}
	if !ValidFolder(folder) {
		return "", "", fmt.Errorf("недопустимая папка редкости %q", folder)
	}
	if !ValidSlug(slug) {
		return "", "", fmt.Errorf("недопустимое имя файла %q", slug)
	}
	if len(plain) == 0 || len(framed) == 0 {
		return "", "", fmt.Errorf("нужны оба файла: без рамки и с рамкой")
	}

	d, err := s.newDriver()
	if err != nil {
		return "", "", err
	}
	defer d.close()

	plainRel, framedRel := rel(PlainDir, folder, slug, ext), rel(FramedDir, folder, slug, ext)

	if !overwrite {
		for _, r := range []string{plainRel, framedRel} {
			ok, err := d.exists(r)
			if err != nil {
				return "", "", err
			}
			if ok {
				return "", "", fmt.Errorf("%w: %s", ErrExists, r)
			}
		}
	}

	if err := d.put(plainRel, plain); err != nil {
		return "", "", fmt.Errorf("без рамки: %w", err)
	}
	// The framed file failing leaves the frameless one behind. That is the
	// recoverable half: the card shows unframed art and the art linter reports
	// the missing counterpart, which is better than losing both.
	if err := d.put(framedRel, framed); err != nil {
		return "", "", fmt.Errorf("с рамкой: %w (файл без рамки уже залит)", err)
	}

	plainURL, framedURL = s.URLs(folder, slug, ext)
	return plainURL, framedURL, nil
}
