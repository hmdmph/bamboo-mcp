package bamboo

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		token     string
		proxyURL  string
		wantError bool
	}{
		{
			name:      "valid client without proxy",
			baseURL:   "https://bamboo.example.com",
			token:     "test-token",
			proxyURL:  "",
			wantError: false,
		},
		{
			name:      "valid client with proxy",
			baseURL:   "https://bamboo.example.com",
			token:     "test-token",
			proxyURL:  "http://proxy:8080",
			wantError: false,
		},
		{
			name:      "invalid proxy URL",
			baseURL:   "https://bamboo.example.com",
			token:     "test-token",
			proxyURL:  "://invalid",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.baseURL, tt.token, tt.proxyURL)
			if tt.wantError {
				if err == nil {
					t.Errorf("NewClient() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewClient() unexpected error: %v", err)
				}
				if client == nil {
					t.Errorf("NewClient() returned nil client")
				}
			}
		})
	}
}
