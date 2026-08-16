package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type BitbucketRepo struct {
	URL      string `json:"url"`
	Username string `json:"username"`
	Token    string `json:"token"`
}

type BitbucketStorage struct {
	mu       sync.RWMutex
	repos    map[string]*BitbucketRepo
	filePath string
}

func NewBitbucketStorage() (*BitbucketStorage, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".bamboo-mcp")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	filePath := filepath.Join(configDir, "bitbucket-repos.json")

	storage := &BitbucketStorage{
		repos:    make(map[string]*BitbucketRepo),
		filePath: filePath,
	}

	if err := storage.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return storage, nil
}

func (s *BitbucketStorage) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &s.repos)
}

func (s *BitbucketStorage) save() error {
	data, err := json.MarshalIndent(s.repos, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal repos: %w", err)
	}

	return os.WriteFile(s.filePath, data, 0600)
}

func (s *BitbucketStorage) AddRepo(name string, repo *BitbucketRepo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.repos[name] = repo
	return s.save()
}

func (s *BitbucketStorage) GetRepo(name string) (*BitbucketRepo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repo, exists := s.repos[name]
	if !exists {
		return nil, fmt.Errorf("repository '%s' not found", name)
	}

	return repo, nil
}

func (s *BitbucketStorage) ListRepos() map[string]*BitbucketRepo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*BitbucketRepo)
	for k, v := range s.repos {
		result[k] = v
	}

	return result
}

func (s *BitbucketStorage) DeleteRepo(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.repos[name]; !exists {
		return fmt.Errorf("repository '%s' not found", name)
	}

	delete(s.repos, name)
	return s.save()
}
