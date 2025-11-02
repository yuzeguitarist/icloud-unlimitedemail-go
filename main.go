package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config 配置结构体
type Config struct {
	// API基础配置
	BaseURL               string `json:"base_url"`
	ClientBuildNumber     string `json:"client_build_number"`
	ClientMasteringNumber string `json:"client_mastering_number"`
	ClientID              string `json:"client_id"`
	DSID                  string `json:"dsid"`

	// 请求头配置
	Headers map[string]string `json:"headers"`

	// 请求体配置
	LangCode string `json:"lang_code"`

	// 批量生成配置
	Count        int `json:"count"`
	DelaySeconds int `json:"delay_seconds"`

	// 邮箱标签配置
	LabelPrefix string `json:"label_prefix"` // 标签前缀，会自动加上序号

	// 输出配置
	OutputFile string `json:"output_file"`

	// 网络配置
	TimeoutSeconds int    `json:"timeout_seconds"`
	UserAgent      string `json:"user_agent"`

	client     *http.Client
	clientOnce sync.Once
}

func (c *Config) httpClient() *http.Client {
	c.clientOnce.Do(func() {
		timeout := c.TimeoutSeconds
		if timeout <= 0 {
			timeout = 30
		}

		if base, ok := http.DefaultTransport.(*http.Transport); ok {
			transport := base.Clone()
			transport.MaxIdleConns = 32
			transport.MaxIdleConnsPerHost = 32
			transport.IdleConnTimeout = 90 * time.Second
			c.client = &http.Client{
				Timeout:   time.Duration(timeout) * time.Second,
				Transport: transport,
			}
			return
		}

		c.client = &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		}
	})

	return c.client
}

func (c *Config) applyRequestHeaders(req *http.Request) {
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}

	if c.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
}

func replaceEndpoint(baseURL, target, replacement string) (string, error) {
	if baseURL == "" {
		return "", fmt.Errorf("基础URL为空，无法构建API端点")
	}

	replaced := strings.Replace(baseURL, target, replacement, 1)
	if replaced == baseURL {
		return "", fmt.Errorf("基础URL %q 未包含期望的路径片段 %q", baseURL, target)
	}

	return replaced, nil
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("无法创建 gzip reader: %w", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("无法读取响应: %w", err)
	}

	return body, nil
}

// GenerateRequest 生成邮箱地址请求体
type GenerateRequest struct {
	LangCode string `json:"langCode"`
}

// GenerateResponse 生成邮箱地址响应体
type GenerateResponse struct {
	Success   bool  `json:"success"`
	Timestamp int64 `json:"timestamp"`
	Result    struct {
		HME string `json:"hme"` // 生成的邮箱地址
	} `json:"result"`
}

// ReserveRequest 确认创建邮箱请求体
type ReserveRequest struct {
	HME   string `json:"hme"`   // 必填：第一步生成的邮箱地址
	Label string `json:"label"` // 必填：邮箱标签/描述
	Note  string `json:"note"`  // 可选：备注
}

// ReserveResponse 创建邮箱响应体
type ReserveResponse struct {
	Success   bool  `json:"success"`
	Timestamp int64 `json:"timestamp"`
	Result    struct {
		HME HMEEmail `json:"hme"`
	} `json:"result"`
	Error *APIError `json:"error,omitempty"`
}

// HMEEmail 邮箱详细信息
type HMEEmail struct {
	Origin          string `json:"origin"`
	AnonymousID     string `json:"anonymousId"`
	Domain          string `json:"domain"`
	HME             string `json:"hme"`
	Label           string `json:"label"`
	Note            string `json:"note"`
	CreateTimestamp int64  `json:"createTimestamp"`
	IsActive        bool   `json:"isActive"`
	RecipientMailID string `json:"recipientMailId"`
	ForwardToEmail  string `json:"forwardToEmail,omitempty"`
}

// ListResponse 邮箱列表响应
type ListResponse struct {
	Success   bool  `json:"success"`
	Timestamp int64 `json:"timestamp"`
	Result    struct {
		ForwardToEmails   []string   `json:"forwardToEmails"`
		HMEEmails         []HMEEmail `json:"hmeEmails"`
		SelectedForwardTo string     `json:"selectedForwardTo"`
	} `json:"result"`
	Error *APIError `json:"error,omitempty"`
}

// DeactivateRequest 删除邮箱请求
type DeactivateRequest struct {
	AnonymousID string `json:"anonymousId"`
}

// DeactivateResponse 删除邮箱响应
type DeactivateResponse struct {
	Success   bool  `json:"success"`
	Timestamp int64 `json:"timestamp"`
	Result    struct {
		Message string `json:"message"`
	} `json:"result"`
	Error *APIError `json:"error,omitempty"`
}

// PermanentDeleteRequest 彻底删除邮箱请求
type PermanentDeleteRequest struct {
	AnonymousID string `json:"anonymousId"`
}

// PermanentDeleteResponse 彻底删除邮箱响应
type PermanentDeleteResponse struct {
	Success   bool  `json:"success"`
	Timestamp int64 `json:"timestamp"`
	Result    struct {
		Message string `json:"message"`
	} `json:"result"`
	Error *APIError `json:"error,omitempty"`
}

// ReactivateRequest 重新激活邮箱请求
type ReactivateRequest struct {
	AnonymousID string `json:"anonymousId"`
}

// ReactivateResponse 重新激活邮箱响应
type ReactivateResponse struct {
	Success   bool  `json:"success"`
	Timestamp int64 `json:"timestamp"`
	Result    struct {
		Message string `json:"message"`
	} `json:"result"`
	Error *APIError `json:"error,omitempty"`
}

// APIError API错误信息
type APIError struct {
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
	RetryAfter   int    `json:"retryAfter"`
}

// 加载配置文件
func loadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("无法打开配置文件: %v", err)
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("无法解析配置文件: %v", err)
	}

	return &config, nil
}

// 第1步：生成邮箱地址
func generateHME(config *Config) (string, error) {
	// 构建 /generate 接口的 URL
	generateURL, err := replaceEndpoint(config.BaseURL, "/reserve", "/generate")
	if err != nil {
		return "", fmt.Errorf("无法构建 generate 接口: %w", err)
	}
	url := fmt.Sprintf("%s?clientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s&dsid=%s",
		generateURL,
		config.ClientBuildNumber,
		config.ClientMasteringNumber,
		config.ClientID,
		config.DSID,
	)

	// 构建请求体
	reqBody := GenerateRequest{
		LangCode: config.LangCode,
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("无法序列化请求体: %v", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("无法创建请求: %v", err)
	}

	config.applyRequestHeaders(req)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// 发送请求
	resp, err := config.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return "", err
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 解析响应
	var response GenerateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("无法解析响应: %v, 原始响应: %s", err, strings.TrimSpace(string(body)))
	}

	// 检查是否成功
	if !response.Success {
		return "", fmt.Errorf("API返回失败: %s", strings.TrimSpace(string(body)))
	}

	return response.Result.HME, nil
}

// 第2步：确认创建邮箱（设置 label）
func reserveHME(config *Config, hme string, label string) (string, error) {
	// 构建 /reserve 接口的 URL
	url := fmt.Sprintf("%s?clientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s&dsid=%s",
		config.BaseURL,
		config.ClientBuildNumber,
		config.ClientMasteringNumber,
		config.ClientID,
		config.DSID,
	)

	// 构建请求体 - 必须包含 hme 和 label
	reqBody := ReserveRequest{
		HME:   hme,   // 第一步生成的邮箱地址
		Label: label, // 邮箱标签
		Note:  "",    // 备注（可选）
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("无法序列化请求体: %v", err)
	}

	// 创建HTTP请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("无法创建请求: %v", err)
	}

	config.applyRequestHeaders(req)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	// 发送请求
	resp, err := config.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return "", err
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// 解析响应
	var response ReserveResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("无法解析响应: %v, 原始响应: %s", err, strings.TrimSpace(string(body)))
	}

	// 检查是否成功
	if !response.Success {
		return "", fmt.Errorf("API返回失败: %s", strings.TrimSpace(string(body)))
	}

	// 返回实际的邮箱地址 - 注意是 result.hme.hme
	return response.Result.HME.HME, nil
}

// 创建隐藏邮件地址（完整流程：生成 + 确认）
func createHME(config *Config, label string) (string, error) {
	// 第1步：生成邮箱地址
	hme, err := generateHME(config)
	if err != nil {
		return "", fmt.Errorf("生成邮箱地址失败: %v", err)
	}

	// 第2步：确认创建并设置 label
	finalHME, err := reserveHME(config, hme, label)
	if err != nil {
		return "", fmt.Errorf("确认创建邮箱失败: %v", err)
	}

	return finalHME, nil
}

// 获取邮箱列表
func listHME(config *Config) ([]HMEEmail, error) {
	// 构建 /list 接口的 URL
	listURL, err := replaceEndpoint(config.BaseURL, "/v1/hme/reserve", "/v2/hme/list")
	if err != nil {
		return nil, fmt.Errorf("无法构建 list 接口: %w", err)
	}
	url := fmt.Sprintf("%s?clientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s&dsid=%s",
		listURL,
		config.ClientBuildNumber,
		config.ClientMasteringNumber,
		config.ClientID,
		config.DSID,
	)

	// 创建HTTP请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("无法创建请求: %v", err)
	}

	config.applyRequestHeaders(req)

	// 发送请求
	resp, err := config.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败: %v", err)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器返回错误 (状态码: %d, 响应: %s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var response ListResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v, 原始响应: %s", err, strings.TrimSpace(string(body)))
	}

	if !response.Success {
		if response.Error != nil {
			return nil, fmt.Errorf("API错误: %s", response.Error.ErrorMessage)
		}
		return nil, fmt.Errorf("获取列表失败")
	}

	return response.Result.HMEEmails, nil
}

// 删除邮箱（停用）
func deactivateHME(config *Config, anonymousID string) error {
	// 构建 /deactivate 接口的 URL
	deactivateURL, err := replaceEndpoint(config.BaseURL, "/reserve", "/deactivate")
	if err != nil {
		return fmt.Errorf("无法构建 deactivate 接口: %w", err)
	}
	url := fmt.Sprintf("%s?clientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s&dsid=%s",
		deactivateURL,
		config.ClientBuildNumber,
		config.ClientMasteringNumber,
		config.ClientID,
		config.DSID,
	)

	// 构建请求体
	reqBody := DeactivateRequest{AnonymousID: anonymousID}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	config.applyRequestHeaders(req)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := config.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %v", err)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误 (状态码: %d, 响应: %s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var response DeactivateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v, 原始响应: %s", err, strings.TrimSpace(string(body)))
	}

	if !response.Success {
		if response.Error != nil {
			return fmt.Errorf("API错误: %s", response.Error.ErrorMessage)
		}
		return fmt.Errorf("停用失败")
	}

	return nil
}

// 彻底删除邮箱（不可恢复）
func permanentDeleteHME(config *Config, anonymousID string) error {
	// 构建 /delete 接口的 URL
	deleteURL, err := replaceEndpoint(config.BaseURL, "/v1/hme/reserve", "/v1/hme/delete")
	if err != nil {
		return fmt.Errorf("无法构建 delete 接口: %w", err)
	}
	url := fmt.Sprintf("%s?clientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s&dsid=%s",
		deleteURL,
		config.ClientBuildNumber,
		config.ClientMasteringNumber,
		config.ClientID,
		config.DSID,
	)

	// 构建请求体
	reqBody := PermanentDeleteRequest{AnonymousID: anonymousID}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	config.applyRequestHeaders(req)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := config.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %v", err)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误 (状态码: %d, 响应: %s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var response PermanentDeleteResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v, 原始响应: %s", err, strings.TrimSpace(string(body)))
	}

	if !response.Success {
		if response.Error != nil {
			return fmt.Errorf("API错误: %s", response.Error.ErrorMessage)
		}
		return fmt.Errorf("彻底删除失败")
	}

	return nil
}

// 重新激活邮箱
func reactivateHME(config *Config, anonymousID string) error {
	// 构建 /reactivate 接口的 URL
	reactivateURL, err := replaceEndpoint(config.BaseURL, "/v1/hme/reserve", "/v1/hme/reactivate")
	if err != nil {
		return fmt.Errorf("无法构建 reactivate 接口: %w", err)
	}
	url := fmt.Sprintf("%s?clientBuildNumber=%s&clientMasteringNumber=%s&clientId=%s&dsid=%s",
		reactivateURL,
		config.ClientBuildNumber,
		config.ClientMasteringNumber,
		config.ClientID,
		config.DSID,
	)

	// 构建请求体
	reqBody := ReactivateRequest{AnonymousID: anonymousID}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("序列化请求失败: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("创建请求失败: %v", err)
	}

	config.applyRequestHeaders(req)
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := config.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %v", err)
	}

	body, err := readResponseBody(resp)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误 (状态码: %d, 响应: %s)", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var response ReactivateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v, 原始响应: %s", err, strings.TrimSpace(string(body)))
	}

	if !response.Success {
		if response.Error != nil {
			return fmt.Errorf("API错误: %s", response.Error.ErrorMessage)
		}
		return fmt.Errorf("重新激活失败")
	}

	return nil
}

// 批量创建邮箱地址
func batchGenerate(config *Config, count int, labelPrefix string) ([]string, []error) {
	if count <= 0 {
		return nil, []error{fmt.Errorf("批量创建数量必须大于 0")}
	}

	emails := make([]string, 0, count)
	errs := make([]error, 0, count)

	printSubHeader("批量创建执行中")
	fmt.Printf("  "+ColorCyan+"数量:"+ColorReset+" %d "+ColorDim+"|"+ColorReset+" "+ColorCyan+"标签:"+ColorReset+" %s*\n\n", count, labelPrefix)

	for i := 0; i < count; i++ {
		label := fmt.Sprintf("%s%d", labelPrefix, i+1)

		// 显示进度条
		printProgressBar(i, count, "创建进度")

		fmt.Printf("  "+ColorGray+"⋯"+ColorReset+" 创建邮箱 "+ColorDim+"(%s)"+ColorReset+" ... ", label)

		email, err := createHME(config, label)
		if err != nil {
			fmt.Printf(ColorRed + "✗" + ColorReset + "\n")
			fmt.Printf("    错误: %v\n", err)
			errs = append(errs, err)
		} else {
			fmt.Printf(ColorGreen + "✓" + ColorReset + "\n")
			fmt.Printf("    "+ColorCyan+"邮箱:"+ColorReset+" %s\n", email)
			emails = append(emails, email)
		}

		// 延迟
		if i < count-1 && config.DelaySeconds > 0 {
			fmt.Printf("    "+ColorDim+"等待 %ds\n"+ColorReset, config.DelaySeconds)
			time.Sleep(time.Duration(config.DelaySeconds) * time.Second)
		}
	}

	// 完成进度条
	printProgressBar(count, count, "创建进度")
	fmt.Println()

	return emails, errs
}

// ANSI 颜色代码 - 丰富多彩配色方案
const (
	ColorReset = "\033[0m"
	ColorBold  = "\033[1m"
	ColorDim   = "\033[2m"

	// 基础颜色 - 大胆使用
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"

	// 亮色版本
	ColorBrightRed     = "\033[91m"
	ColorBrightGreen   = "\033[92m"
	ColorBrightYellow  = "\033[93m"
	ColorBrightBlue    = "\033[94m"
	ColorBrightMagenta = "\033[95m"
	ColorBrightCyan    = "\033[96m"
	ColorBrightWhite   = "\033[97m"

	// 灰色系
	ColorGray      = "\033[90m"
	ColorLightGray = "\033[37m"

	// 背景色
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
)

// UI 辅助函数 - 多彩风格
func printSeparator() {
	fmt.Println(ColorCyan + strings.Repeat("─", 70) + ColorReset)
}

func printThickSeparator() {
	fmt.Println(ColorBrightCyan + strings.Repeat("━", 70) + ColorReset)
}

func printHeader(title string) {
	fmt.Println()
	printThickSeparator()
	fmt.Printf(ColorBold+"  %s"+ColorReset+"\n", title)
	printThickSeparator()
	fmt.Println()
}

func printSubHeader(title string) {
	fmt.Println()
	fmt.Printf(ColorBold+ColorBrightBlue+"┌─ %s"+ColorReset+"\n", title)
	printSeparator()
}

func printSuccess(message string) {
	fmt.Printf(ColorGreen+"  ✓"+ColorReset+" %s\n", message)
}

func printError(message string) {
	fmt.Printf(ColorRed+"  ✗"+ColorReset+" %s\n", message)
}

func printWarning(message string) {
	fmt.Printf(ColorYellow+"  !"+ColorReset+" %s\n", message)
}

func printInfo(message string) {
	fmt.Printf("  "+ColorCyan+"›"+ColorReset+" %s\n", message)
}

func printStep(message string) {
	fmt.Printf("  "+ColorDim+"⋯"+ColorReset+" %s\n", message)
}

func printProgressBar(current, total int, prefix string) {
	barWidth := 40
	if total <= 0 {
		total = 1
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}

	progress := float64(current) / float64(total)
	filled := int(progress * float64(barWidth))

	if filled > barWidth {
		filled = barWidth
	}

	// 彩色渐变进度条
	var bar strings.Builder
	bar.WriteString(ColorBrightWhite + "[" + ColorReset)
	for i := 0; i < barWidth; i++ {
		if i < filled {
			// 根据进度使用不同颜色
			if progress < 0.3 {
				bar.WriteString(ColorBrightRed + "█" + ColorReset)
			} else if progress < 0.7 {
				bar.WriteString(ColorBrightYellow + "█" + ColorReset)
			} else {
				bar.WriteString(ColorBrightGreen + "█" + ColorReset)
			}
		} else {
			bar.WriteString(ColorGray + "░" + ColorReset)
		}
	}
	bar.WriteString(ColorBrightWhite + "]" + ColorReset)

	percentage := int(progress * 100)
	if percentage < 0 {
		percentage = 0
	} else if percentage > 100 {
		percentage = 100
	}

	fmt.Printf("\r  "+ColorBrightCyan+"%s"+ColorReset+" %s "+ColorBold+ColorBrightMagenta+"%3d%%"+ColorReset+" "+ColorBlue+"(%d/%d)"+ColorReset,
		prefix, bar.String(), percentage, current, total)

	if current == total {
		fmt.Println()
	}
}

func withSpinner(message string, action func() error) (err error) {
	// 彩色加载动画
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	colors := []string{ColorBrightCyan, ColorBrightBlue, ColorBrightMagenta, ColorBrightRed, ColorBrightYellow, ColorBrightGreen}

	if len(frames) == 0 {
		return action()
	}

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()
		idx := 0
		frameCount := len(frames)
		colorCount := len(colors)
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				frame := frames[idx%frameCount]
				color := ColorBrightWhite
				if colorCount > 0 {
					color = colors[idx%colorCount]
				}
				fmt.Printf("\r  "+color+"%s"+ColorReset+" "+ColorBrightWhite+"%s"+ColorReset, frame, message)
				idx++
			}
		}
	}()

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("执行过程中出现未知错误: %v", r)
		}

		close(done)
		wg.Wait()

		statusColor := ColorBrightGreen
		statusSymbol := "✓"
		statusText := ColorGreen + "完成" + ColorReset
		if err != nil {
			statusColor = ColorBrightRed
			statusSymbol = "✗"
			statusText = ColorRed + "失败" + ColorReset
		}

		fmt.Printf("\r  %s%s"+ColorReset+" "+ColorBrightWhite+"%s"+ColorReset+" %s  \n",
			statusColor, statusSymbol, message, statusText)
	}()

	err = action()
	return err
}

func readInput(prompt string) string {
	fmt.Print(ColorCyan + "  › " + ColorReset + prompt)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return strings.TrimSpace(input)
		}
		fmt.Println()
		printError(fmt.Sprintf("读取输入失败: %v", err))
		return ""
	}
	return strings.TrimSpace(input)
}

func readInt(prompt string) (int, error) {
	input := readInput(prompt)
	if input == "" {
		return 0, fmt.Errorf("请输入有效的数字")
	}
	return strconv.Atoi(input)
}

func confirmAction(message string) bool {
	fmt.Printf("\n  "+ColorYellow+"?"+ColorReset+" %s "+ColorDim+"(y/n)"+ColorReset+": ", message)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	// 支持多种确认方式
	return input == "y" || input == "yes" || input == "是"
}

// 保存邮箱到文件
func saveEmailsToFile(emails []string, filename string) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		printError(fmt.Sprintf("无法打开文件: %v", err))
		return
	}
	defer file.Close()

	for _, email := range emails {
		_, err := file.WriteString(email + "\n")
		if err != nil {
			printError(fmt.Sprintf("写入失败: %v", err))
			return
		}
	}

	printSuccess(fmt.Sprintf("已保存到 %s", filename))
}

// 显示主菜单
func showMainMenu() {
	printHeader("iCloud 隐藏邮箱管理工具")

	fmt.Println("  " + ColorGreen + "[1]" + ColorReset + " 查看邮箱列表")
	fmt.Println("  " + ColorBlue + "[2]" + ColorReset + " 创建新邮箱")
	fmt.Println("  " + ColorYellow + "[3]" + ColorReset + " 停用邮箱")
	fmt.Println("  " + ColorMagenta + "[4]" + ColorReset + " 批量创建邮箱")
	fmt.Println("  " + ColorRed + "[5]" + ColorReset + " 彻底删除停用的邮箱 " + ColorDim + "(不可恢复)" + ColorReset)
	fmt.Println("  " + ColorCyan + "[6]" + ColorReset + " 重新激活停用的邮箱")
	fmt.Println("  " + ColorDim + "[0]" + ColorReset + " 退出 " + ColorDim + "(或输入 q/quit/exit)" + ColorReset)

	printSeparator()
	fmt.Println()
}

// 查看邮箱列表
func handleListEmails(config *Config) {
	printHeader("邮箱列表")
	var emails []HMEEmail
	if err := withSpinner("获取邮箱列表", func() error {
		var err error
		emails, err = listHME(config)
		return err
	}); err != nil {
		printError(fmt.Sprintf("获取列表失败: %v", err))
		return
	}

	if len(emails) == 0 {
		printInfo("暂无邮箱")
		return
	}

	// 统计邮箱状态
	activeCount := 0
	deactivatedCount := 0
	for _, email := range emails {
		if email.IsActive {
			activeCount++
		} else {
			deactivatedCount++
		}
	}

	fmt.Printf("  "+ColorBold+"总计"+ColorReset+" %d "+ColorDim+"|"+ColorReset+" "+ColorGreen+"激活"+ColorReset+" %d "+ColorDim+"|"+ColorReset+" "+ColorYellow+"停用"+ColorReset+" %d\n\n",
		len(emails), activeCount, deactivatedCount)

	for i, email := range emails {
		var statusSymbol, emailColor string
		if email.IsActive {
			statusSymbol = ColorBrightGreen + "●" + ColorReset
			emailColor = ColorBrightWhite
		} else {
			statusSymbol = ColorYellow + "○" + ColorReset
			emailColor = ColorGray
		}

		fmt.Printf("  "+ColorBrightCyan+"%2d."+ColorReset+" %s "+emailColor+"%s"+ColorReset+"\n", i+1, statusSymbol, email.HME)
		fmt.Printf("      "+ColorBrightBlue+"Ἷ7 标签: "+ColorReset+ColorCyan+"%s"+ColorReset+"\n", email.Label)

		if email.ForwardToEmail != "" {
			fmt.Printf("      "+ColorBrightMagenta+"➤ 转发: "+ColorReset+ColorMagenta+"%s"+ColorReset+"\n", email.ForwardToEmail)
		}

		// 显示创建时间
		createTime := time.Unix(email.CreateTimestamp/1000, 0)
		fmt.Printf("      "+ColorBrightGreen+"⏰ 创建: "+ColorReset+ColorGreen+"%s"+ColorReset+"\n", createTime.Format("2006-01-02 15:04"))
		fmt.Println()
	}
}

// 创建单个邮箱
func handleCreateEmail(config *Config) {
	printHeader("创建新邮箱")

	label := readInput("邮箱标签: ")
	if label == "" {
		printError("标签不能为空")
		return
	}

	var email string
	if err := withSpinner("创建邮箱", func() error {
		var err error
		email, err = createHME(config, label)
		return err
	}); err != nil {
		printError(fmt.Sprintf("创建失败: %v", err))
		return
	}

	fmt.Println()
	printSuccess("邮箱创建成功")
	fmt.Printf("\n  "+ColorBrightMagenta+"✉ 邮箱: "+ColorReset+ColorBold+ColorBrightWhite+"%s"+ColorReset+"\n", email)
	fmt.Printf("  "+ColorBrightBlue+"Ἷ7 标签: "+ColorReset+ColorCyan+"%s"+ColorReset+"\n", label)
	fmt.Printf("  "+ColorBrightGreen+"⏰ 时间: "+ColorReset+ColorGreen+"%s"+ColorReset+"\n", time.Now().Format("2006-01-02 15:04"))
}

// 停用邮箱
func handleDeleteEmails(config *Config) {
	printHeader("停用邮箱")
	var emails []HMEEmail
	if err := withSpinner("正在获取邮箱列表", func() error {
		var err error
		emails, err = listHME(config)
		return err
	}); err != nil {
		printError(fmt.Sprintf("获取邮箱列表失败: %v", err))
		return
	}

	// 筛选出激活的邮箱
	var activeEmails []HMEEmail
	for _, email := range emails {
		if email.IsActive {
			activeEmails = append(activeEmails, email)
		}
	}

	if len(activeEmails) == 0 {
		printWarning("暂无激活的邮箱")
		return
	}

	fmt.Printf("  "+ColorBold+"激活邮箱"+ColorReset+" "+ColorGreen+"%d 个"+ColorReset+"\n\n", len(activeEmails))

	for i, email := range activeEmails {
		fmt.Printf("  "+ColorDim+"%2d."+ColorReset+" "+ColorGreen+"●"+ColorReset+" %s\n", i+1, email.HME)
		fmt.Printf("      "+ColorCyan+"标签:"+ColorReset+" %s\n", email.Label)
		fmt.Println()
	}

	printInfo("输入序号 (逗号分隔，如: 1,3,5)")
	input := readInput("序号: ")

	if input == "" {
		printInfo("已取消")
		return
	}

	// 解析序号
	parts := strings.Split(input, ",")
	var toDeactivate []HMEEmail

	for _, part := range parts {
		idx, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || idx < 1 || idx > len(activeEmails) {
			printError(fmt.Sprintf("无效的序号: %s", part))
			return
		}
		toDeactivate = append(toDeactivate, activeEmails[idx-1])
	}

	// 显示将要停用的邮箱
	fmt.Printf("\n  "+ColorBold+"将停用"+ColorReset+" "+ColorYellow+"%d 个邮箱"+ColorReset+"\n\n", len(toDeactivate))
	for _, email := range toDeactivate {
		fmt.Printf("  "+ColorYellow+"›"+ColorReset+" %s "+ColorDim+"(%s)"+ColorReset+"\n", email.HME, email.Label)
	}

	printInfo("停用后可重新激活")
	if !confirmAction("确认停用这些邮箱") {
		printInfo("已取消")
		return
	}

	// 执行停用
	printSubHeader("执行停用")
	successCount := 0
	failCount := 0

	for i, email := range toDeactivate {
		printProgressBar(i, len(toDeactivate), "停用进度")
		fmt.Printf("  "+ColorDim+"⋯"+ColorReset+" 停用 %s ... ", email.HME)

		err := deactivateHME(config, email.AnonymousID)
		if err != nil {
			fmt.Printf(ColorRed + "✗" + ColorReset + "\n")
			fmt.Printf("    错误: %v\n", err)
			failCount++
		} else {
			fmt.Printf(ColorGreen + "✓" + ColorReset + "\n")
			successCount++
		}

		if i < len(toDeactivate)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 完成进度条
	printProgressBar(len(toDeactivate), len(toDeactivate), "停用进度")

	fmt.Println()
	printSeparator()
	if successCount > 0 {
		printSuccess(fmt.Sprintf("成功停用 %d 个", successCount))
	}
	if failCount > 0 {
		printError(fmt.Sprintf("失败 %d 个", failCount))
	}
}

// 批量创建邮箱
func handleBatchCreate(config *Config) {
	printHeader("批量创建邮箱")

	count, err := readInt("创建数量: ")
	if err != nil || count <= 0 {
		printError("数量无效，请输入大于 0 的整数")
		return
	}

	if count > 50 {
		printWarning("建议单次创建不超过 50 个")
		if !confirmAction("继续创建这么多邮箱") {
			printInfo("已取消")
			return
		}
	}

	labelPrefix := readInput("标签前缀 " + ColorGray + "(默认: auto-)" + ColorReset + ": ")
	if labelPrefix == "" {
		labelPrefix = "auto-"
	}

	fmt.Printf("\n  " + ColorBold + "创建计划" + ColorReset + "\n\n")
	fmt.Printf("  "+ColorCyan+"数量:"+ColorReset+" "+ColorBold+"%d"+ColorReset+" 个\n", count)
	fmt.Printf("  "+ColorCyan+"标签:"+ColorReset+" %s1, %s2, %s3, ...\n", labelPrefix, labelPrefix, labelPrefix)
	fmt.Printf("  "+ColorCyan+"延迟:"+ColorReset+" %d 秒\n", config.DelaySeconds)

	estimatedTime := count * config.DelaySeconds
	fmt.Printf("  "+ColorDim+"耗时: %d:%02d"+ColorReset+"\n", estimatedTime/60, estimatedTime%60)

	if !confirmAction("开始批量创建") {
		printInfo("已取消")
		return
	}

	emails, errors := batchGenerate(config, count, labelPrefix)

	printSeparator()
	if len(emails) > 0 {
		printSuccess(fmt.Sprintf("批量创建完成 (成功 %d 个)", len(emails)))
	}
	if len(errors) > 0 {
		printError(fmt.Sprintf("失败 %d 个", len(errors)))
	}

	if len(emails) > 0 {
		fmt.Println("\n  " + ColorBold + "创建结果" + ColorReset)
		fmt.Println()
		for i, email := range emails {
			fmt.Printf("  "+ColorDim+"%2d."+ColorReset+" "+ColorGreen+"✓"+ColorReset+" %s\n", i+1, email)
		}

		// 保存到文件
		if config.OutputFile != "" {
			fmt.Println()
			saveEmailsToFile(emails, config.OutputFile)
		}
	}
}

// 彻底删除停用的邮箱
func handlePermanentDelete(config *Config) {
	printHeader("彻底删除停用的邮箱（不可恢复！）")
	printWarning("此操作将永久删除邮箱，无法恢复！")

	var emails []HMEEmail
	if err := withSpinner("正在获取邮箱列表", func() error {
		var err error
		emails, err = listHME(config)
		return err
	}); err != nil {
		printError(fmt.Sprintf("获取邮箱列表失败: %v", err))
		return
	}

	// 筛选出已停用的邮箱
	var deactivatedEmails []HMEEmail
	for _, email := range emails {
		if !email.IsActive {
			deactivatedEmails = append(deactivatedEmails, email)
		}
	}

	if len(deactivatedEmails) == 0 {
		printWarning("暂无已停用的邮箱")
		return
	}

	fmt.Printf("  "+ColorBold+"已停用邮箱"+ColorReset+" %d 个\n\n", len(deactivatedEmails))

	for i, email := range deactivatedEmails {
		fmt.Printf("  "+ColorGray+"%2d."+ColorReset+" "+ColorGray+"○"+ColorReset+" %s\n", i+1, email.HME)
		fmt.Printf("      "+ColorGray+"标签: "+ColorReset+"%s\n", email.Label)
		fmt.Println()
	}

	printInfo("输入序号 (逗号分隔，如: 1,3,5)")
	input := readInput("序号: ")

	if input == "" {
		printInfo("已取消")
		return
	}

	// 解析序号
	parts := strings.Split(input, ",")
	var toDelete []HMEEmail

	for _, part := range parts {
		idx, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || idx < 1 || idx > len(deactivatedEmails) {
			printError(fmt.Sprintf("无效的序号: %s", part))
			return
		}
		toDelete = append(toDelete, deactivatedEmails[idx-1])
	}

	// 显示将要删除的邮箱
	fmt.Printf("\n  "+ColorBold+ColorRed+"彻底删除"+ColorReset+" %d 个邮箱\n\n", len(toDelete))
	for _, email := range toDelete {
		fmt.Printf("  "+ColorRed+"›"+ColorReset+" %s "+ColorDim+"(%s)"+ColorReset+"\n", email.HME, email.Label)
	}

	printWarning("此操作不可恢复")
	fmt.Print("\n  " + ColorYellow + "?" + ColorReset + " 确认删除? 请输入 " + ColorBold + "DELETE" + ColorReset + ": ")
	reader := bufio.NewReader(os.Stdin)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(confirm)

	if confirm != "DELETE" {
		printInfo("已取消")
		return
	}

	// 执行彻底删除
	printSubHeader("执行删除")
	successCount := 0
	failCount := 0

	for i, email := range toDelete {
		printProgressBar(i, len(toDelete), "删除进度")
		fmt.Printf("  "+ColorDim+"⋯"+ColorReset+" 删除 %s ... ", email.HME)

		err := permanentDeleteHME(config, email.AnonymousID)
		if err != nil {
			fmt.Printf(ColorRed + "✗" + ColorReset + "\n")
			fmt.Printf("    错误: %v\n", err)
			failCount++
		} else {
			fmt.Printf(ColorGreen + "✓" + ColorReset + "\n")
			successCount++
		}

		if i < len(toDelete)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 完成进度条
	printProgressBar(len(toDelete), len(toDelete), "删除进度")

	fmt.Println()
	printSeparator()
	if successCount > 0 {
		printSuccess(fmt.Sprintf("成功删除 %d 个", successCount))
	}
	if failCount > 0 {
		printError(fmt.Sprintf("失败 %d 个", failCount))
	}
}

// 重新激活停用的邮箱
func handleReactivate(config *Config) {
	printHeader("重新激活停用的邮箱")
	var emails []HMEEmail
	if err := withSpinner("正在获取邮箱列表", func() error {
		var err error
		emails, err = listHME(config)
		return err
	}); err != nil {
		printError(fmt.Sprintf("获取邮箱列表失败: %v", err))
		return
	}

	// 筛选出已停用的邮箱
	var deactivatedEmails []HMEEmail
	for _, email := range emails {
		if !email.IsActive {
			deactivatedEmails = append(deactivatedEmails, email)
		}
	}

	if len(deactivatedEmails) == 0 {
		printWarning("暂无已停用的邮箱")
		return
	}

	fmt.Printf("  "+ColorBold+"已停用邮箱"+ColorReset+" %d 个\n\n", len(deactivatedEmails))

	for i, email := range deactivatedEmails {
		fmt.Printf("  "+ColorGray+"%2d."+ColorReset+" "+ColorGray+"○"+ColorReset+" %s\n", i+1, email.HME)
		fmt.Printf("      "+ColorGray+"标签: "+ColorReset+"%s\n", email.Label)
		fmt.Println()
	}

	printInfo("输入序号 (逗号分隔，如: 1,3,5)")
	input := readInput("序号: ")

	if input == "" {
		printInfo("已取消")
		return
	}

	// 解析序号
	parts := strings.Split(input, ",")
	var toReactivate []HMEEmail

	for _, part := range parts {
		idx, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || idx < 1 || idx > len(deactivatedEmails) {
			printError(fmt.Sprintf("无效的序号: %s", part))
			return
		}
		toReactivate = append(toReactivate, deactivatedEmails[idx-1])
	}

	// 显示将要重新激活的邮箱
	fmt.Printf("\n  "+ColorBold+"将激活"+ColorReset+" "+ColorGreen+"%d 个邮箱"+ColorReset+"\n\n", len(toReactivate))
	for _, email := range toReactivate {
		fmt.Printf("  "+ColorGreen+"›"+ColorReset+" %s "+ColorDim+"(%s)"+ColorReset+"\n", email.HME, email.Label)
	}

	if !confirmAction("确认重新激活这些邮箱") {
		printInfo("已取消")
		return
	}

	// 执行重新激活
	printSubHeader("执行激活")
	successCount := 0
	failCount := 0

	for i, email := range toReactivate {
		printProgressBar(i, len(toReactivate), "激活进度")
		fmt.Printf("  "+ColorDim+"⋯"+ColorReset+" 激活 %s ... ", email.HME)

		err := reactivateHME(config, email.AnonymousID)
		if err != nil {
			fmt.Printf(ColorRed + "✗" + ColorReset + "\n")
			fmt.Printf("    错误: %v\n", err)
			failCount++
		} else {
			fmt.Printf(ColorGreen + "✓" + ColorReset + "\n")
			successCount++
		}

		if i < len(toReactivate)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 完成进度条
	printProgressBar(len(toReactivate), len(toReactivate), "激活进度")

	fmt.Println()
	printSeparator()
	if successCount > 0 {
		printSuccess(fmt.Sprintf("成功激活 %d 个", successCount))
	}
	if failCount > 0 {
		printError(fmt.Sprintf("失败 %d 个", failCount))
	}
}

func main() {
	// 显示启动信息
	printHeader("iCloud 隐藏邮箱管理工具")
	fmt.Printf("  " + ColorCyan + "版本:" + ColorReset + " " + ColorBold + "v2.0" + ColorReset + "\n")
	fmt.Printf("  " + ColorCyan + "作者:" + ColorReset + " yuzeguitarist\n")
	fmt.Println()

	// 加载配置
	var config *Config
	if err := withSpinner("加载配置文件", func() error {
		var err error
		config, err = loadConfig("config.json")
		return err
	}); err != nil {
		printError(fmt.Sprintf("加载失败: %v", err))
		printInfo("请确保 config.json 文件存在且格式正确")
		os.Exit(1)
	}

	// 主循环
	for {
		showMainMenu()
		choice := readInput("选择操作 (0-6): ")
		choice = strings.ToLower(strings.TrimSpace(choice))

		switch choice {
		case "1", "l", "list":
			handleListEmails(config)
		case "2", "c", "create":
			handleCreateEmail(config)
		case "3", "d", "deactivate":
			handleDeleteEmails(config)
		case "4", "b", "batch":
			handleBatchCreate(config)
		case "5", "delete":
			handlePermanentDelete(config)
		case "6", "r", "reactivate":
			handleReactivate(config)
		case "0", "q", "quit", "exit", "e":
			fmt.Println()
			printThickSeparator()
			fmt.Printf("  感谢使用\n")
			printThickSeparator()
			return
		default:
			printError("无效选择，请输入 0-6 或对应字母")
		}

		fmt.Print("\n  " + ColorDim + "按回车键继续..." + ColorReset)
		readInput("")

		// 清屏效果
		fmt.Print("\033[2J\033[H")
	}
}
