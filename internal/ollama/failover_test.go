package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEndpointPoolClientUsesFirstHealthyEndpoint(t *testing.T) {
	t.Parallel()

	primary := newChatTestServer(t, http.StatusOK, "primary")
	defer primary.Close()

	secondary := newChatTestServer(t, http.StatusOK, "secondary")
	defer secondary.Close()

	client := NewEndpointPoolClient([]*Client{
		NewClient(Config{BaseURL: primary.URL, Model: "test", TimeoutSec: 2}, nil),
		NewClient(Config{BaseURL: secondary.URL, Model: "test", TimeoutSec: 2}, nil),
	}, time.Minute, nil)

	reply, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if reply != "primary" {
		t.Fatalf("expected primary reply, got %q", reply)
	}
}

func TestEndpointPoolClientFallsThroughMultipleBackups(t *testing.T) {
	t.Parallel()

	primary := newChatTestServer(t, http.StatusBadGateway, "primary-down")
	defer primary.Close()

	secondary := newChatTestServer(t, http.StatusBadGateway, "secondary-down")
	defer secondary.Close()

	tertiary := newChatTestServer(t, http.StatusOK, "tertiary")
	defer tertiary.Close()

	client := NewEndpointPoolClient([]*Client{
		NewClient(Config{BaseURL: primary.URL, Model: "test", TimeoutSec: 2}, nil),
		NewClient(Config{BaseURL: secondary.URL, Model: "test", TimeoutSec: 2}, nil),
		NewClient(Config{BaseURL: tertiary.URL, Model: "test", TimeoutSec: 2}, nil),
	}, time.Minute, nil)

	reply, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if reply != "tertiary" {
		t.Fatalf("expected tertiary reply, got %q", reply)
	}
}

func TestEndpointPoolClientSkipsKnownUnhealthyEndpointOnNextRequest(t *testing.T) {
	t.Parallel()

	primaryStatuses := []int{http.StatusBadGateway, http.StatusOK}
	primary := newSequencedChatTestServer(t, primaryStatuses, "primary")
	defer primary.Close()

	secondary := newChatTestServer(t, http.StatusOK, "secondary")
	defer secondary.Close()

	client := NewEndpointPoolClient([]*Client{
		NewClient(Config{BaseURL: primary.URL, Model: "test", TimeoutSec: 2}, nil),
		NewClient(Config{BaseURL: secondary.URL, Model: "test", TimeoutSec: 2}, nil),
	}, time.Minute, nil)

	reply, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("first Chat returned error: %v", err)
	}
	if reply != "secondary" {
		t.Fatalf("expected first reply from secondary, got %q", reply)
	}

	reply, err = client.Chat(context.Background(), []Message{{Role: "user", Content: "hello again"}})
	if err != nil {
		t.Fatalf("second Chat returned error: %v", err)
	}
	if reply != "secondary" {
		t.Fatalf("expected second reply from secondary while primary unhealthy, got %q", reply)
	}
}

func TestEndpointPoolClientSupportsOpenAICompatibleEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(OpenAIChatResponse{
				Model: "mlx-test",
				Choices: []OpenAIChatChoice{{
					Index: 0,
					Message: OpenAIChatMessage{
						Role:    "assistant",
						Content: "mlx-ok",
					},
				}},
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"mlx-test"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewEndpointPoolClient([]*Client{
		NewClient(Config{BaseURL: server.URL + "/v1", Model: "mlx-test", TimeoutSec: 2}, nil),
	}, time.Minute, nil)

	reply, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if reply != "mlx-ok" {
		t.Fatalf("expected mlx-ok reply, got %q", reply)
	}
}

func newChatTestServer(t *testing.T, status int, content string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			if status >= 400 {
				http.Error(w, content, status)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(ChatResponse{
				Message: Message{Role: "assistant", Content: content},
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/api/tags":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newSequencedChatTestServer(t *testing.T, statuses []int, content string) *httptest.Server {
	t.Helper()

	var idx int
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/chat":
			status := statuses[idx]
			if idx < len(statuses)-1 {
				idx++
			}
			if status >= 400 {
				http.Error(w, content, status)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(ChatResponse{
				Message: Message{Role: "assistant", Content: content},
			}); err != nil {
				t.Fatalf("encode response: %v", err)
			}
		case "/api/tags":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}
