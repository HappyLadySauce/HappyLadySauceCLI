// Package metadata resolves model capabilities from OpenAI-compatible providers.
// Package metadata 从 OpenAI 兼容服务商解析模型能力信息。
package metadata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// SourceProviderModels marks metadata resolved from the provider /models endpoint.
	// SourceProviderModels 表示元数据来自服务商 /models 端点。
	SourceProviderModels = "provider_models"
)

// ModelMetadata describes runtime model limits and capabilities.
// ModelMetadata 描述运行时模型限制与能力。
type ModelMetadata struct {
	ID                string
	OwnedBy           string
	MaxContextTokens  int
	MaxOutputTokens   int
	SupportsReasoning bool
	SupportsImageIn   bool
	SupportsVideoIn   bool
	Source            string
}

// Resolver fetches metadata from an OpenAI-compatible /models endpoint.
// Resolver 从 OpenAI 兼容 /models 端点获取模型元数据。
type Resolver struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// ResolverConfig groups provider connection settings for metadata resolution.
// ResolverConfig 聚合元数据解析所需的服务商连接配置。
type ResolverConfig struct {
	BaseURL    string
	APIKey     string
	HTTPClient *http.Client
}

// NewResolver creates a provider metadata resolver.
// NewResolver 创建服务商模型元数据解析器。
func NewResolver(cfg ResolverConfig) (*Resolver, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		return nil, errors.New("metadata resolver base URL is required")
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &Resolver{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(cfg.APIKey),
		httpClient: client,
	}, nil
}

// Resolve fetches model metadata for modelName. Missing optional fields are left zero-valued.
// Resolve 获取 modelName 的模型元数据；缺失的可选字段保持零值。
func (r *Resolver) Resolve(ctx context.Context, modelName string) (*ModelMetadata, error) {
	if r == nil {
		return nil, errors.New("metadata resolver is nil")
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return nil, errors.New("metadata model name is required")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("build metadata request: %w", err)
	}
	if r.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+r.apiKey)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request model metadata: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("request model metadata: unexpected status %d", resp.StatusCode)
	}

	var body modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode model metadata: %w", err)
	}
	for _, item := range body.Data {
		if strings.TrimSpace(item.ID) != modelName {
			continue
		}
		return modelFromRaw(item), nil
	}
	return nil, fmt.Errorf("model metadata for %q not found", modelName)
}

type modelsResponse struct {
	Data []rawModel `json:"data"`
}

type rawModel struct {
	ID      string         `json:"id"`
	Object  string         `json:"object"`
	OwnedBy string         `json:"owned_by"`
	Created int64          `json:"created"`
	Raw     map[string]any `json:"-"`
}

func (m *rawModel) UnmarshalJSON(data []byte) error {
	type alias rawModel
	var value alias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*m = rawModel(value)
	m.Raw = raw
	return nil
}

func modelFromRaw(item rawModel) *ModelMetadata {
	return &ModelMetadata{
		ID:                item.ID,
		OwnedBy:           item.OwnedBy,
		MaxContextTokens:  firstInt(item.Raw, "context_length", "max_context_length", "context_window", "max_model_len"),
		MaxOutputTokens:   firstInt(item.Raw, "max_output_tokens", "max_completion_tokens", "output_context_length"),
		SupportsReasoning: boolField(item.Raw, "supports_reasoning"),
		SupportsImageIn:   boolField(item.Raw, "supports_image_in"),
		SupportsVideoIn:   boolField(item.Raw, "supports_video_in"),
		Source:            SourceProviderModels,
	}
}

func firstInt(raw map[string]any, keys ...string) int {
	for _, key := range keys {
		if value := intField(raw, key); value > 0 {
			return value
		}
	}
	return 0
}

func intField(raw map[string]any, key string) int {
	switch value := raw[key].(type) {
	case float64:
		if value > 0 {
			return int(value)
		}
	case int:
		if value > 0 {
			return value
		}
	case json.Number:
		n, _ := value.Int64()
		if n > 0 {
			return int(n)
		}
	case string:
		var n int
		if _, err := fmt.Sscanf(value, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

func boolField(raw map[string]any, key string) bool {
	value, _ := raw[key].(bool)
	return value
}
