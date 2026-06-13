package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/HappyLadySauce/HappyLadySauceCLI/internal/capability"
	securitycore "github.com/HappyLadySauce/HappyLadySauceCLI/internal/security"
)

// https://uapis.cn/docs/api-reference/get-misc-weather
var weatherAPIURL = "https://uapis.cn/api/v1/misc/weather"

// weatherHTTPClient caps outbound weather API latency to avoid hung tool calls.
// weatherHTTPClient 限制天气 API 出站请求耗时，避免 tool 调用永久阻塞。
var weatherHTTPClient = &http.Client{Timeout: 10 * time.Second}

// maxResponseBytes caps the weather API response body size to avoid unbounded reads.
// maxResponseBytes 限制天气 API 响应体大小，避免无界读取。
const maxResponseBytes = 1 << 20 // 1 MiB

type WeatherToolParams struct {
	City string `json:"city" jsonschema:"description=字符类型, 城市名称, 示例: 北京, required"`
	Lang string `json:"lang" jsonschema:"description=字符类型, 语言,必须是以下之一: zh, en, optional"`
}

type WeatherToolResult struct {
	Province      string `json:"province"`
	City          string `json:"city"`
	Adcode        string `json:"adcode"`
	Weather       string `json:"weather"`
	WeatherIcon   string `json:"weather_icon"`
	Temperature   int    `json:"temperature"`
	WindDirection string `json:"wind_direction"`
	WindPower     string `json:"wind_power"`
	Humidity      int    `json:"humidity"`
	ReportTime    string `json:"report_time"`
}

// GetWeather fetches current weather for the given city via uapis.cn.
// GetWeather 通过 uapis.cn 获取指定城市的当前天气。
func getWeather(ctx context.Context, req *WeatherToolParams) (*WeatherToolResult, error) {
	if req == nil {
		return nil, fmt.Errorf("weather request is nil")
	}

	city := strings.TrimSpace(req.City)
	if city == "" {
		return nil, fmt.Errorf("city is required")
	}

	lang, err := normalizeLang(req.Lang)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	query.Set("city", city)
	query.Set("lang", lang)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, weatherAPIURL+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("create weather request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/json")

	resp, err := weatherHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("call weather API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("weather API returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read weather response: %w", err)
	}

	var result WeatherToolResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode weather response: %w", err)
	}

	return &result, nil
}

// normalizeLang validates and defaults the response language.
// normalizeLang 校验响应语言参数并在缺省时使用中文。
func normalizeLang(lang string) (string, error) {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return "zh", nil
	}
	if lang != "zh" && lang != "en" {
		return "", fmt.Errorf("lang must be zh or en")
	}
	return lang, nil
}

func GetWeatherTool() (tool.InvokableTool, error) {
	t, err := utils.InferTool(
		"get_weather",
		"天气工具, 获取指定城市的天气信息",
		getWeather,
	)
	if err != nil {
		return nil, fmt.Errorf("infer get_weather tool: %w", err)
	}
	return t, nil
}

// CapabilityDescriptor returns the security metadata for get_weather.
// CapabilityDescriptor 返回 get_weather 的安全元数据。
func CapabilityDescriptor() capability.Descriptor {
	return capability.Descriptor{
		Name:          "get_weather",
		Type:          capability.TypeNativeTool,
		Source:        capability.SourceBuiltin,
		Risk:          capability.RiskLow,
		DefaultPolicy: capability.DefaultPolicyAllow,
		Scopes:        []string{"network:weather"},
		Resources:     []string{"https://uapis.cn/api/v1/misc/weather"},
	}
}

// OperationBuilder returns the operation metadata builder for get_weather calls.
// OperationBuilder 返回 get_weather 调用的操作元数据构建器。
func OperationBuilder() securitycore.OperationBuilder {
	return func(ctx context.Context, request securitycore.OperationRequest, input securitycore.OperationBuildInput) (securitycore.OperationRequest, error) {
		request.OperationKind = "network.weather"
		request.Resources = []securitycore.OperationResource{
			{Kind: securitycore.ResourceKindURL, Value: weatherAPIURL},
		}
		request.SanitizedArgsSummary = input.Summary
		return request, nil
	}
}
