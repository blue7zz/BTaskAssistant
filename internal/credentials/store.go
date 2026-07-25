package credentials

import (
	"errors"

	"github.com/zalando/go-keyring"
)

var ErrNotFound = errors.New("凭据不存在")

// Store keeps secrets outside the SQLite workspace snapshot. The production
// implementation delegates to macOS Keychain, Windows Credential Manager, or
// Linux Secret Service through go-keyring.
type Store interface {
	Set(account string, secret string) error
	Get(account string) (string, error)
	Delete(account string) error
}

type SystemStore struct {
	service string
}

func NewSystemStore(service string) *SystemStore {
	return &SystemStore{service: service}
}

func (s *SystemStore) Set(account string, secret string) error {
	return keyring.Set(s.service, account, secret)
}

func (s *SystemStore) Get(account string) (string, error) {
	secret, err := keyring.Get(s.service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return secret, err
}

func (s *SystemStore) Delete(account string) error {
	err := keyring.Delete(s.service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
