package model

import (
	"context"
	"errors"
	"testing"
)

// fakeClient is a minimal Client used for factory registration tests.
type fakeClient struct{ provider string }

func (f *fakeClient) Chat(_ context.Context, _ ChatRequest) (Response, error) {
	return Response{}, nil
}
func (f *fakeClient) ChatStream(_ context.Context, _ ChatRequest) (<-chan Chunk, error) {
	ch := make(chan Chunk, 1)
	ch <- Chunk{Done: true}
	close(ch)
	return ch, nil
}
func (f *fakeClient) Provider() string { return f.provider }

func init() {
	RegisterProvider("fakeprovider", func(mc ModelConfig, apiKey string) (Client, error) {
		return &fakeClient{provider: "fakeprovider"}, nil
	})
}

func TestFactory_NoAPIKey(t *testing.T) {
	cfg := &Config{
		Models: []ModelConfig{
			{
				ID:        "cloud-model",
				Provider:  "fakeprovider",
				APIKeyEnv: "FAKE_API_KEY_THAT_IS_NOT_SET_XYZ",
			},
		},
	}

	t.Setenv("FAKE_API_KEY_THAT_IS_NOT_SET_XYZ", "")

	_, err := NewClient(cfg, "cloud-model")
	if err == nil {
		t.Fatal("NewClient: expected ErrNoAPIKey, got nil")
	}
	if !errors.Is(err, ErrNoAPIKey) {
		t.Errorf("NewClient: got %v, want ErrNoAPIKey", err)
	}
	// Error message must mention the env var name so the user knows what to set.
	if err.Error() == "" {
		t.Error("NewClient: error message is empty")
	}
	const envVar = "FAKE_API_KEY_THAT_IS_NOT_SET_XYZ"
	if !containsString(err.Error(), envVar) {
		t.Errorf("NewClient: error %q does not mention env var %q", err.Error(), envVar)
	}
}

func TestFactory_ModelNotFound(t *testing.T) {
	cfg := &Config{Models: []ModelConfig{}}
	_, err := NewClient(cfg, "nonexistent")
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("NewClient: got %v, want ErrModelNotFound", err)
	}
}

func TestFactory_LocalProvider_NoKeyRequired(t *testing.T) {
	RegisterProvider("localtest", func(mc ModelConfig, apiKey string) (Client, error) {
		return &fakeClient{provider: "localtest"}, nil
	})

	cfg := &Config{
		Models: []ModelConfig{
			{
				ID:       "local-model",
				Provider: "localtest",
				// No APIKeyEnv — local providers don't need a key.
			},
		},
	}

	client, err := NewClient(cfg, "local-model")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client.Provider() != "localtest" {
		t.Errorf("Provider: got %q, want localtest", client.Provider())
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsSubstring(s, sub))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
