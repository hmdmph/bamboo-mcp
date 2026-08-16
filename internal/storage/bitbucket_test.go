package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBitbucketStorage(t *testing.T) {
	tmpDir := t.TempDir()

	storage := &BitbucketStorage{
		repos:    make(map[string]*BitbucketRepo),
		filePath: filepath.Join(tmpDir, "test-repos.json"),
	}

	t.Run("AddRepo", func(t *testing.T) {
		repo := &BitbucketRepo{
			URL:      "https://bitbucket.org/test/repo",
			Username: "testuser",
			Token:    "testtoken",
		}

		err := storage.AddRepo("test-repo", repo)
		if err != nil {
			t.Errorf("AddRepo() error = %v", err)
		}

		if _, err := os.Stat(storage.filePath); os.IsNotExist(err) {
			t.Errorf("Storage file was not created")
		}
	})

	t.Run("GetRepo", func(t *testing.T) {
		repo, err := storage.GetRepo("test-repo")
		if err != nil {
			t.Errorf("GetRepo() error = %v", err)
		}
		if repo.URL != "https://bitbucket.org/test/repo" {
			t.Errorf("GetRepo() URL = %v, want %v", repo.URL, "https://bitbucket.org/test/repo")
		}
	})

	t.Run("ListRepos", func(t *testing.T) {
		repos := storage.ListRepos()
		if len(repos) != 1 {
			t.Errorf("ListRepos() count = %v, want 1", len(repos))
		}
	})

	t.Run("DeleteRepo", func(t *testing.T) {
		err := storage.DeleteRepo("test-repo")
		if err != nil {
			t.Errorf("DeleteRepo() error = %v", err)
		}

		_, err = storage.GetRepo("test-repo")
		if err == nil {
			t.Errorf("GetRepo() should return error for deleted repo")
		}
	})

	t.Run("GetNonExistentRepo", func(t *testing.T) {
		_, err := storage.GetRepo("non-existent")
		if err == nil {
			t.Errorf("GetRepo() should return error for non-existent repo")
		}
	})

	t.Run("DeleteNonExistentRepo", func(t *testing.T) {
		err := storage.DeleteRepo("non-existent")
		if err == nil {
			t.Errorf("DeleteRepo() should return error for non-existent repo")
		}
	})
}
