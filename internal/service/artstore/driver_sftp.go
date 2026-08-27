package artstore

import (
	"errors"
	"fmt"
	"net"
	"path"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const sftpTimeout = 20 * time.Second

// sftpStore writes to the art host over SSH.
type sftpStore struct {
	client *sftp.Client
	conn   *ssh.Client
	root   string
}

func newSFTP(cfg Config) (driver, error) {
	if cfg.SFTPUser == "" || cfg.SFTPPassword == "" {
		return nil, fmt.Errorf("не заданы ART_SFTP_USER / ART_SFTP_PASSWORD")
	}
	if cfg.SFTPRoot == "" {
		return nil, fmt.Errorf("не задан ART_SFTP_ROOT")
	}
	hostKey, err := parseHostKey(cfg.SFTPHostKey)
	if err != nil {
		return nil, err
	}

	addr := cfg.SFTPHost
	if !strings.Contains(addr, ":") {
		addr += ":22"
	}

	conn, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            cfg.SFTPUser,
		Auth:            []ssh.AuthMethod{ssh.Password(cfg.SFTPPassword)},
		HostKeyCallback: hostKey,
		Timeout:         sftpTimeout,
	})
	if err != nil {
		return nil, fmt.Errorf("ssh: %w", err)
	}

	client, err := sftp.NewClient(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("sftp: %w", err)
	}
	return &sftpStore{client: client, conn: conn, root: strings.TrimRight(cfg.SFTPRoot, "/")}, nil
}

// parseHostKey builds the host key check. There is deliberately no way to skip
// it: the password would be handed to whatever answers on that address.
func parseHostKey(raw string) (ssh.HostKeyCallback, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf(
			"не задан ART_SFTP_HOST_KEY — сними его командой `ssh-keyscan -t ed25519 <хост>` " +
				"и положи строку целиком, иначе пароль уйдёт непроверенному серверу")
	}

	// A "SHA256:…" fingerprint, as printed by ssh-keygen -lf.
	if strings.HasPrefix(raw, "SHA256:") {
		want := raw
		return func(_ string, _ net.Addr, key ssh.PublicKey) error {
			if got := ssh.FingerprintSHA256(key); got != want {
				return fmt.Errorf("отпечаток ключа хоста не совпал: получен %s, ожидался %s", got, want)
			}
			return nil
		}, nil
	}

	// Otherwise an ssh-keyscan line, with or without the leading host field.
	fields := strings.Fields(raw)
	if len(fields) >= 3 {
		raw = strings.Join(fields[1:], " ")
	}
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("ART_SFTP_HOST_KEY не разобран: %w", err)
	}
	return ssh.FixedHostKey(key), nil
}

func (s *sftpStore) full(rel string) string { return path.Join(s.root, rel) }

func (s *sftpStore) exists(rel string) (bool, error) {
	_, err := s.client.Stat(s.full(rel))
	if err == nil {
		return true, nil
	}
	if isNotExist(err) {
		return false, nil
	}
	return false, err
}

func (s *sftpStore) put(rel string, data []byte) error {
	full := s.full(rel)
	if err := s.client.MkdirAll(path.Dir(full)); err != nil {
		return fmt.Errorf("mkdir %s: %w", path.Dir(full), err)
	}

	// Upload beside the target and rename, so the web server never serves a
	// half-written file. Rename does not overwrite on all servers, so an
	// intentional overwrite removes the old file first.
	tmp := full + ".part"
	f, err := s.client.Create(tmp)
	if err != nil {
		return fmt.Errorf("create %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		s.client.Remove(tmp)
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		s.client.Remove(tmp)
		return err
	}
	if _, err := s.client.Stat(full); err == nil {
		if err := s.client.Remove(full); err != nil {
			s.client.Remove(tmp)
			return fmt.Errorf("remove %s: %w", full, err)
		}
	}
	if err := s.client.Rename(tmp, full); err != nil {
		s.client.Remove(tmp)
		return fmt.Errorf("rename %s: %w", full, err)
	}
	return nil
}

func isNotExist(err error) bool {
	if err == nil {
		return false
	}
	var status *sftp.StatusError
	if errors.As(err, &status) {
		return status.FxCode() == sftp.ErrSSHFxNoSuchFile
	}
	return strings.Contains(strings.ToLower(err.Error()), "file does not exist")
}

func (s *sftpStore) describe() string { return "sftp " + s.root }

func (s *sftpStore) close() error {
	if s.client != nil {
		s.client.Close()
	}
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}
