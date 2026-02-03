package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetFluxAPI_Success(t *testing.T) {
	responseJSON := `{"code": 0, "message": "success", "data": {}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 验证请求方法
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// 验证 Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "test-token" {
			t.Errorf("Expected Authorization 'test-token', got '%s'", auth)
		}

		// 验证 Content-Type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(responseJSON))
	}))
	defer server.Close()

	ctx := context.Background()
	body, err := GetFluxAPI(ctx, server.URL, "test-token")
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if string(body) != responseJSON {
		t.Errorf("Expected response '%s', got '%s'", responseJSON, string(body))
	}
}

func TestGetFluxAPI_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	ctx := context.Background()
	_, err := GetFluxAPI(ctx, server.URL, "bad-token")
	if err == nil {
		t.Fatal("Expected error for 401 response, got nil")
	}
}

func TestGetFluxAPI_NetworkError(t *testing.T) {
	ctx := context.Background()
	_, err := GetFluxAPI(ctx, "http://localhost:1", "token")
	if err == nil {
		t.Fatal("Expected error for network failure, got nil")
	}
}
