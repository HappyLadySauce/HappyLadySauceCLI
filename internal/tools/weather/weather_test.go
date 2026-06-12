package weather

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetWeather_Beijing(t *testing.T) {
	resp, err := getWeather(context.Background(), &WeatherToolParams{
		City: "北京",
		Lang: "zh",
	})
	if err != nil {
		t.Fatalf("getWeather returned error: %v", err)
	}
	if resp.City == "" {
		t.Fatalf("expected non-empty city, got %+v", resp)
	}
	if resp.Temperature == 0 && resp.Weather == "" {
		t.Fatalf("expected weather data, got %+v", resp)
	}
}

func TestGetWeather_Validation(t *testing.T) {
	_, err := getWeather(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request")
	}

	_, err = getWeather(context.Background(), &WeatherToolParams{City: "  "})
	if err == nil {
		t.Fatal("expected error for empty city")
	}

	_, err = getWeather(context.Background(), &WeatherToolParams{City: "北京", Lang: "fr"})
	if err == nil {
		t.Fatal("expected error for invalid lang")
	}
}

func TestGetWeatherTool(t *testing.T) {
	tool, err := GetWeatherTool()
	if err != nil {
		t.Fatalf("GetWeatherTool() error = %v", err)
	}
	if tool == nil {
		t.Fatal("expected non-nil weather tool")
	}

	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatalf("tool.Info returned error: %v", err)
	}
	if info.Name != "get_weather" {
		t.Fatalf("unexpected tool name: %s", info.Name)
	}
}

func TestGetWeather_HTTPErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failure", http.StatusInternalServerError)
	}))
	defer server.Close()

	originalURL := weatherAPIURL
	weatherAPIURL = server.URL
	t.Cleanup(func() { weatherAPIURL = originalURL })

	_, err := getWeather(context.Background(), &WeatherToolParams{
		City: "北京",
		Lang: "zh",
	})
	if err == nil {
		t.Fatal("expected error for non-2xx weather response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status code in error, got: %v", err)
	}
}
