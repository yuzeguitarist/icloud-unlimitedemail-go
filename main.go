/*
MIT License

Copyright (c) 2025 yuzeguitarist

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package main

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
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

	// 并发配置
	MaxConcurrency int `json:"max_concurrency"` // 最大并发数，0表示串行

	// 邮箱标签配置
	LabelPrefix string `json:"label_prefix"` // 标签前缀，会自动加上序号

	// 输出配置
	OutputFile string `json:"output_file"`

	// 网络配置
	TimeoutSeconds int    `json:"timeout_seconds"`
	UserAgent      string `json:"user_agent"`

	// 邮箱质量评估配置
	EmailQuality EmailQualityConfig `json:"email_quality"`

	// 邮箱保存配置
	SaveGeneratedEmails bool   `json:"save_generated_emails"` // 是否保存生成的邮箱列表
	EmailListFile       string `json:"email_list_file"`       // 邮箱列表保存文件

	// 开发者模式
	DeveloperMode bool `json:"developer_mode"` // 开发者模式，显示调试功能

	client     *http.Client
	clientOnce sync.Once
}

// ConfigManager 配置管理器
type ConfigManager struct {
	config     *Config
	configPath string
	mutex      sync.RWMutex
	callbacks  []func(*Config)
	lastMod    time.Time
}

// ProcessSafetyManager 进程安全管理器
type ProcessSafetyManager struct {
	lockFile   string
	isLocked   bool
	mutex      sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	operations sync.WaitGroup
}

// NetworkManager 网络管理器
type NetworkManager struct {
	client     *http.Client
	retryCount int
	timeout    time.Duration
	mutex      sync.Mutex
}

// 全局管理器实例
var (
	configManager  *ConfigManager
	safetyManager  *ProcessSafetyManager
	networkManager *NetworkManager
	globalConfig   *Config
	configMutex    sync.RWMutex
)

// 程序常量
const (
	VERSION     = "v2.3.0"
	AUTHOR      = "yuzeguitarist"
	LOCK_FILE   = ".icloud_smart.lock"
	CONFIG_FILE = "config.json"
)

// EmailQualityConfig 邮箱质量评估配置
type EmailQualityConfig struct {
	// 自动选择配置
	AutoSelect         bool `json:"auto_select"`          // 是否自动选择最佳邮箱
	MinScore           int  `json:"min_score"`            // 最低接受分数 (0-100)
	MaxRegenerateCount int  `json:"max_regenerate_count"` // 最大重新生成次数

	// 手动选择配置
	ShowScores    bool `json:"show_scores"`     // 是否显示邮箱分数
	AllowManual   bool `json:"allow_manual"`    // 是否允许手动选择
	ShowAllEmails bool `json:"show_all_emails"` // 是否显示所有生成的邮箱

	// 评分权重配置
	Weights ScoreWeights `json:"weights"`
}

// ScoreWeights 评分权重配置
type ScoreWeights struct {
	PrefixStructure int `json:"prefix_structure"` // 前缀结构权重 (0-100)
	Length          int `json:"length"`           // 长度权重 (0-100)
	Readability     int `json:"readability"`      // 可读性权重 (0-100)
	Security        int `json:"security"`         // 安全性权重 (0-100)
}

// EmailCandidate 邮箱候选项
type EmailCandidate struct {
	Email string `json:"email"`
	Score int    `json:"score"`
	ID    int    `json:"id"` // 生成顺序ID (1, 2, 3)
}

// ConfigManager 方法实现

// NewConfigManager 创建新的配置管理器
func NewConfigManager(configPath string) *ConfigManager {
	return &ConfigManager{
		configPath: configPath,
		callbacks:  make([]func(*Config), 0),
	}
}

// LoadConfig 加载配置文件
func (cm *ConfigManager) LoadConfig() (*Config, error) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	data, err := os.ReadFile(cm.configPath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %v", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %v", err)
	}

	// 设置默认值
	cm.setDefaults(&config)

	cm.config = &config

	// 获取文件修改时间
	if stat, err := os.Stat(cm.configPath); err == nil {
		cm.lastMod = stat.ModTime()
	}

	return &config, nil
}

// SaveConfig 保存配置文件
func (cm *ConfigManager) SaveConfig(config *Config) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(cm.configPath, data, 0644); err != nil {
		return fmt.Errorf("保存配置文件失败: %v", err)
	}

	cm.config = config

	// 更新修改时间
	if stat, err := os.Stat(cm.configPath); err == nil {
		cm.lastMod = stat.ModTime()
	}

	// 通知回调函数
	for _, callback := range cm.callbacks {
		callback(config)
	}

	return nil
}

// CheckForUpdates 检查配置文件是否有更新
func (cm *ConfigManager) CheckForUpdates() bool {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	stat, err := os.Stat(cm.configPath)
	if err != nil {
		return false
	}

	return stat.ModTime().After(cm.lastMod)
}

// GetConfig 获取当前配置
func (cm *ConfigManager) GetConfig() *Config {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()
	return cm.config
}

// AddCallback 添加配置更新回调
func (cm *ConfigManager) AddCallback(callback func(*Config)) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()
	cm.callbacks = append(cm.callbacks, callback)
}

// setDefaults 设置默认值
func (cm *ConfigManager) setDefaults(config *Config) {
	if config.TimeoutSeconds == 0 {
		config.TimeoutSeconds = 30
	}
	if config.DelaySeconds == 0 {
		config.DelaySeconds = 1
	}
	if config.Count == 0 {
		config.Count = 1
	}
	if config.EmailQuality.MinScore == 0 {
		config.EmailQuality.MinScore = 70
	}
	if config.EmailQuality.MaxRegenerateCount == 0 {
		config.EmailQuality.MaxRegenerateCount = 3
	}
	if config.EmailQuality.Weights.PrefixStructure == 0 {
		config.EmailQuality.Weights.PrefixStructure = 40
	}
	if config.EmailQuality.Weights.Length == 0 {
		config.EmailQuality.Weights.Length = 20
	}
	if config.EmailQuality.Weights.Readability == 0 {
		config.EmailQuality.Weights.Readability = 25
	}
	if config.EmailQuality.Weights.Security == 0 {
		config.EmailQuality.Weights.Security = 15
	}
	if config.EmailListFile == "" {
		config.EmailListFile = "generated_emails.txt"
	}
	// DeveloperMode 默认为 false，不需要设置
}

// ProcessSafetyManager 方法实现

// NewProcessSafetyManager 创建进程安全管理器
func NewProcessSafetyManager() *ProcessSafetyManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ProcessSafetyManager{
		lockFile: LOCK_FILE,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Lock 获取进程锁
func (psm *ProcessSafetyManager) Lock() error {
	psm.mutex.Lock()
	defer psm.mutex.Unlock()

	if psm.isLocked {
		return nil
	}

	// 检查锁文件是否存在
	if _, err := os.Stat(psm.lockFile); err == nil {
		// 读取PID
		data, err := os.ReadFile(psm.lockFile)
		if err == nil {
			pid := strings.TrimSpace(string(data))
			return fmt.Errorf("程序已在运行 (PID: %s)", pid)
		}
	}

	// 创建锁文件
	pid := fmt.Sprintf("%d", os.Getpid())
	if err := os.WriteFile(psm.lockFile, []byte(pid), 0644); err != nil {
		return fmt.Errorf("创建锁文件失败: %v", err)
	}

	psm.isLocked = true
	return nil
}

// Unlock 释放进程锁
func (psm *ProcessSafetyManager) Unlock() error {
	psm.mutex.Lock()
	defer psm.mutex.Unlock()

	if !psm.isLocked {
		return nil
	}

	// 等待所有操作完成
	psm.operations.Wait()

	// 删除锁文件
	if err := os.Remove(psm.lockFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("删除锁文件失败: %v", err)
	}

	psm.isLocked = false
	psm.cancel()
	return nil
}

// AddOperation 添加操作计数
func (psm *ProcessSafetyManager) AddOperation() {
	psm.operations.Add(1)
}

// DoneOperation 完成操作计数
func (psm *ProcessSafetyManager) DoneOperation() {
	psm.operations.Done()
}

// Context 获取上下文
func (psm *ProcessSafetyManager) Context() context.Context {
	return psm.ctx
}

// NetworkManager 方法实现

// NewNetworkManager 创建网络管理器
func NewNetworkManager(timeout time.Duration, retryCount int) *NetworkManager {
	return &NetworkManager{
		timeout:    timeout,
		retryCount: retryCount,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetClient 获取HTTP客户端
func (nm *NetworkManager) GetClient() *http.Client {
	nm.mutex.Lock()
	defer nm.mutex.Unlock()
	return nm.client
}

// DoWithRetry 带重试的HTTP请求（使用指数退避策略）
func (nm *NetworkManager) DoWithRetry(req *http.Request) (*http.Response, error) {
	var lastErr error
	baseDelay := 500 * time.Millisecond // 基础延迟 500ms

	for i := 0; i <= nm.retryCount; i++ {
		if i > 0 {
			// 指数退避: 500ms, 1s, 2s, 4s, 8s...
			delay := baseDelay * time.Duration(1<<uint(i-1))
			// 最大延迟不超过 10 秒
			if delay > 10*time.Second {
				delay = 10 * time.Second
			}
			time.Sleep(delay)
		}

		resp, err := nm.client.Do(req)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// 检查是否是网络错误
		if isNetworkError(err) {
			continue
		}

		// 非网络错误直接返回
		break
	}

	return nil, fmt.Errorf("请求失败 (重试%d次): %v", nm.retryCount, lastErr)
}

// isNetworkError 判断是否是网络错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()
	return strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable")
}

// EmailQualityResult 邮箱质量评估结果
type EmailQualityResult struct {
	Candidates   []EmailCandidate `json:"candidates"`
	BestEmail    string           `json:"best_email"`
	BestScore    int              `json:"best_score"`
	TotalTries   int              `json:"total_tries"`
	AutoSelected bool             `json:"auto_selected"`
}

func (c *Config) httpClient() *http.Client {
	c.clientOnce.Do(func() {
		timeout := c.TimeoutSeconds
		if timeout <= 0 {
			timeout = 30
		}

		// 优化的 HTTP 传输配置
		transport := &http.Transport{
			// 连接池优化
			MaxIdleConns:        100,              // 全局最大空闲连接数
			MaxIdleConnsPerHost: 10,               // 每个主机最大空闲连接数
			MaxConnsPerHost:     0,                // 每个主机最大连接数（0表示不限制）
			IdleConnTimeout:     90 * time.Second, // 空闲连接超时

			// 连接建立超时优化
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second, // 连接超时
				KeepAlive: 30 * time.Second, // TCP KeepAlive
			}).DialContext,

			// 响应头超时
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,

			// TLS 优化
			TLSHandshakeTimeout: 10 * time.Second,

			// 启用 HTTP/2
			ForceAttemptHTTP2: true,

			// 禁用压缩（我们已有 gzip 处理）
			DisableCompression: false,
		}

		c.client = &http.Client{
			Timeout:   time.Duration(timeout) * time.Second,
			Transport: transport,
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

	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("无法解析基础URL %q: %w", baseURL, err)
	}

	normalizePath := func(p string) string {
		if p == "" {
			return ""
		}
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
		cleaned := path.Clean(p)
		if cleaned == "." {
			return ""
		}
		return cleaned
	}

	currentPath := parsedURL.Path
	if currentPath == "" {
		currentPath = "/"
	}
	currentPath = path.Clean(currentPath)
	if !strings.HasPrefix(currentPath, "/") {
		currentPath = "/" + currentPath
	}

	targetPath := normalizePath(target)
	if targetPath == "" {
		return "", fmt.Errorf("目标路径为空，无法构建API端点")
	}
	replacementPath := normalizePath(replacement)
	if replacementPath == "" {
		return "", fmt.Errorf("替换路径为空，无法构建API端点")
	}

	updatedPath := strings.Replace(currentPath, targetPath, replacementPath, 1)
	if updatedPath == currentPath {
		return "", fmt.Errorf("基础URL %q 未包含期望的路径片段 %q", baseURL, targetPath)
	}

	parsedURL.Path = updatedPath
	return parsedURL.String(), nil
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
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
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

// 邮箱质量评估算法
func evaluateEmailQuality(email string, weights ScoreWeights) int {
	if email == "" {
		return 0
	}

	// 分离前缀和域名
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return 0
	}
	prefix := parts[0]
	domain := parts[1]

	var totalScore float64
	var totalWeight int

	// 1. 前缀结构评分 (0-100)
	if weights.PrefixStructure > 0 {
		structureScore := evaluatePrefixStructure(prefix)
		totalScore += float64(structureScore * weights.PrefixStructure)
		totalWeight += weights.PrefixStructure
	}

	// 2. 长度评分 (0-100)
	if weights.Length > 0 {
		lengthScore := evaluateLength(prefix)
		totalScore += float64(lengthScore * weights.Length)
		totalWeight += weights.Length
	}

	// 3. 可读性评分 (0-100)
	if weights.Readability > 0 {
		readabilityScore := evaluateReadability(prefix)
		totalScore += float64(readabilityScore * weights.Readability)
		totalWeight += weights.Readability
	}

	// 4. 安全性评分 (0-100)
	if weights.Security > 0 {
		securityScore := evaluateSecurity(prefix, domain)
		totalScore += float64(securityScore * weights.Security)
		totalWeight += weights.Security
	}

	if totalWeight == 0 {
		return 0
	}

	// 计算加权平均分
	finalScore := int(totalScore / float64(totalWeight))
	if finalScore > 100 {
		finalScore = 100
	}
	if finalScore < 0 {
		finalScore = 0
	}

	return finalScore
}

// 评估前缀结构 (0-100分)
func evaluatePrefixStructure(prefix string) int {
	if prefix == "" {
		return 0
	}

	// 纯字母 - 最安全 (90-100分)
	if isOnlyLetters(prefix) {
		if len(prefix) >= 4 && len(prefix) <= 12 {
			return 95
		}
		return 85
	}

	// 字母+点号 - 次优选择 (70-85分)
	if isLettersWithDots(prefix) {
		dotCount := strings.Count(prefix, ".")
		if dotCount == 1 && len(prefix) >= 5 && len(prefix) <= 15 {
			return 80
		}
		if dotCount <= 2 {
			return 70
		}
		return 50 // 太多点号
	}

	// 字母+数字 - 可接受 (60-75分)
	if isLettersWithNumbers(prefix) {
		digitCount := countDigits(prefix)
		if digitCount <= 4 && len(prefix) >= 4 && len(prefix) <= 15 {
			return 65
		}
		return 55
	}

	// 包含下划线或连字符 - 较差 (30-50分)
	if strings.Contains(prefix, "_") || strings.Contains(prefix, "-") {
		underscoreCount := strings.Count(prefix, "_")
		hyphenCount := strings.Count(prefix, "-")
		if underscoreCount+hyphenCount == 1 {
			return 45
		}
		return 25 // 多个特殊字符
	}

	// 其他复杂格式 - 很差 (0-30分)
	return 20
}

// 评估长度 (0-100分)
func evaluateLength(prefix string) int {
	length := len(prefix)

	// 理想长度 6-10 字符 (90-100分)
	if length >= 6 && length <= 10 {
		return 95
	}

	// 可接受长度 4-5 或 11-12 字符 (70-85分)
	if (length >= 4 && length <= 5) || (length >= 11 && length <= 12) {
		return 75
	}

	// 较短或较长 3 或 13-15 字符 (50-65分)
	if length == 3 || (length >= 13 && length <= 15) {
		return 55
	}

	// 太短或太长 (0-40分)
	if length <= 2 {
		return 10
	}
	if length >= 16 {
		return 30
	}

	return 40
}

// 评估可读性 (0-100分)
func evaluateReadability(prefix string) int {
	if prefix == "" {
		return 0
	}

	score := 50 // 基础分

	// 检查是否像真实单词
	if looksLikeRealWords(prefix) {
		score += 30
	}

	// 检查字符重复
	if hasExcessiveRepeating(prefix) {
		score -= 25
	}

	// 检查随机性
	if looksRandom(prefix) {
		score -= 30
	}

	// 检查元音辅音比例
	if hasGoodVowelConsonantRatio(prefix) {
		score += 15
	}

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// 评估安全性 (0-100分)
func evaluateSecurity(prefix, domain string) int {
	score := 50 // 基础分

	// 域名评分
	switch domain {
	case "icloud.com":
		score += 25 // iCloud 域名很好
	case "gmail.com":
		score += 30 // Gmail 域名最好
	case "outlook.com", "hotmail.com":
		score += 20
	default:
		score += 10 // 其他域名
	}

	// 检查是否看起来像临时邮箱
	if looksLikeTemporaryEmail(prefix) {
		score -= 30
	}

	// 检查是否包含明显的无限邮箱特征
	if hasInfiniteEmailPattern(prefix) {
		score -= 25
	}

	// 检查特殊字符过多
	specialCharCount := countSpecialChars(prefix)
	if specialCharCount > 2 {
		score -= 20
	}

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	return score
}

// 辅助函数：检查是否只包含字母
func isOnlyLetters(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')) {
			return false
		}
	}
	return len(s) > 0
}

// 辅助函数：检查是否是字母+点号的组合
func isLettersWithDots(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '.') {
			return false
		}
	}
	return len(s) > 0 && strings.Contains(s, ".")
}

// 辅助函数：检查是否是字母+数字的组合
func isLettersWithNumbers(s string) bool {
	hasLetter := false
	hasDigit := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			hasLetter = true
		} else if r >= '0' && r <= '9' {
			hasDigit = true
		} else {
			return false
		}
	}
	return hasLetter && hasDigit
}

// 辅助函数：计算数字字符数量
func countDigits(s string) int {
	count := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			count++
		}
	}
	return count
}

// 辅助函数：检查是否看起来像真实单词
func looksLikeRealWords(s string) bool {
	// 简单的启发式检查
	s = strings.ToLower(s)

	// 常见的英文单词模式
	commonPatterns := []string{
		"john", "smith", "mike", "david", "alex", "chris", "sarah", "mary",
		"test", "demo", "user", "admin", "mail", "email", "work", "home",
		"info", "contact", "support", "hello", "world", "apple", "google",
	}

	for _, pattern := range commonPatterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}

	// 检查元音辅音模式
	vowels := "aeiou"
	consonants := "bcdfghjklmnpqrstvwxyz"

	vowelCount := 0
	consonantCount := 0

	for _, r := range s {
		if strings.ContainsRune(vowels, r) {
			vowelCount++
		} else if strings.ContainsRune(consonants, r) {
			consonantCount++
		}
	}

	// 合理的元音辅音比例
	if vowelCount > 0 && consonantCount > 0 {
		ratio := float64(vowelCount) / float64(consonantCount)
		return ratio >= 0.2 && ratio <= 2.0
	}

	return false
}

// 辅助函数：检查是否有过多重复字符
func hasExcessiveRepeating(s string) bool {
	if len(s) < 2 {
		return false
	}

	maxRepeat := 0
	currentRepeat := 1

	for i := 1; i < len(s); i++ {
		if s[i] == s[i-1] {
			currentRepeat++
		} else {
			if currentRepeat > maxRepeat {
				maxRepeat = currentRepeat
			}
			currentRepeat = 1
		}
	}

	if currentRepeat > maxRepeat {
		maxRepeat = currentRepeat
	}

	return maxRepeat >= 3 // 连续3个或以上相同字符
}

// 辅助函数：检查是否看起来随机
func looksRandom(s string) bool {
	if len(s) < 4 {
		return false
	}

	// 检查字符变化频率
	changes := 0
	for i := 1; i < len(s); i++ {
		if s[i] != s[i-1] {
			changes++
		}
	}

	changeRatio := float64(changes) / float64(len(s)-1)

	// 如果变化太频繁，可能是随机字符串
	if changeRatio > 0.8 {
		return true
	}

	// 检查是否包含常见的随机字符串模式
	randomPatterns := []string{
		"xyz", "abc", "123", "qwe", "asd", "zxc",
	}

	s = strings.ToLower(s)
	for _, pattern := range randomPatterns {
		if strings.Contains(s, pattern) {
			return true
		}
	}

	return false
}

// 辅助函数：检查元音辅音比例是否合理
func hasGoodVowelConsonantRatio(s string) bool {
	vowels := "aeiouAEIOU"
	vowelCount := 0
	consonantCount := 0

	for _, r := range s {
		if strings.ContainsRune(vowels, r) {
			vowelCount++
		} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			consonantCount++
		}
	}

	if vowelCount == 0 || consonantCount == 0 {
		return false
	}

	ratio := float64(vowelCount) / float64(consonantCount)
	return ratio >= 0.25 && ratio <= 1.5
}

// 辅助函数：检查是否看起来像临时邮箱
func looksLikeTemporaryEmail(prefix string) bool {
	prefix = strings.ToLower(prefix)

	// 临时邮箱常见模式
	tempPatterns := []string{
		"temp", "tmp", "test", "fake", "dummy", "throw", "disposable",
		"10min", "guerrilla", "mailinator", "tempmail", "yopmail",
		"random", "generated", "auto", "spam", "junk",
	}

	for _, pattern := range tempPatterns {
		if strings.Contains(prefix, pattern) {
			return true
		}
	}

	// 检查是否全是数字或看起来像随机生成
	if len(prefix) >= 6 {
		digitCount := countDigits(prefix)
		if float64(digitCount)/float64(len(prefix)) > 0.6 {
			return true
		}
	}

	return false
}

// 辅助函数：检查是否有无限邮箱模式
func hasInfiniteEmailPattern(prefix string) bool {
	// 检查是否包含 + 号（虽然iCloud不支持，但作为检查）
	if strings.Contains(prefix, "+") {
		return true
	}

	// 检查是否有明显的无限邮箱标识
	infinitePatterns := []string{
		"unlimited", "infinite", "forever", "noreply", "donotreply",
		"plus", "alias", "forward", "redirect",
	}

	prefix = strings.ToLower(prefix)
	for _, pattern := range infinitePatterns {
		if strings.Contains(prefix, pattern) {
			return true
		}
	}

	return false
}

// 辅助函数：计算特殊字符数量
func countSpecialChars(s string) int {
	count := 0
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.') {
			count++
		}
	}
	return count
}

// 智能邮箱生成器 - 核心功能（并发优化版本）
func generateSmartEmail(config *Config, label string) (*EmailQualityResult, error) {
	qualityConfig := config.EmailQuality
	maxTries := qualityConfig.MaxRegenerateCount
	if maxTries <= 0 {
		maxTries = 3 // 默认最多3次
	}

	printSubHeader("智能邮箱生成")
	fmt.Printf("  "+ColorCyan+"目标分数:"+ColorReset+" %d+ "+ColorDim+"|"+ColorReset+" "+ColorCyan+"最大尝试:"+ColorReset+" %d 次\n\n", qualityConfig.MinScore, maxTries)

	// 并发生成所有候选邮箱
	type candidateResult struct {
		candidate EmailCandidate
		err       error
	}

	resultChan := make(chan candidateResult, maxTries)
	var wg sync.WaitGroup

	for i := 1; i <= maxTries; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// 生成邮箱
			email, err := generateHME(config)
			if err != nil {
				resultChan <- candidateResult{err: err}
				return
			}

			// 评估质量
			score := evaluateEmailQuality(email, qualityConfig.Weights)
			resultChan <- candidateResult{
				candidate: EmailCandidate{
					Email: email,
					Score: score,
					ID:    id,
				},
			}
		}(i)
	}

	// 等待所有任务完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	var candidates []EmailCandidate
	var bestEmail string
	var bestScore int

	for result := range resultChan {
		if result.err != nil {
			fmt.Printf("  "+ColorRed+"[!]"+ColorReset+" 生成失败: %v\n", result.err)
			continue
		}

		candidate := result.candidate
		candidates = append(candidates, candidate)

		// 显示结果
		var scoreColor string
		if candidate.Score >= qualityConfig.MinScore {
			scoreColor = ColorGreen
		} else if candidate.Score >= qualityConfig.MinScore-20 {
			scoreColor = ColorYellow
		} else {
			scoreColor = ColorRed
		}

		fmt.Printf("  "+ColorGreen+"[+]"+ColorReset+" 邮箱 #%d: %s\n", candidate.ID, candidate.Email)
		fmt.Printf("      "+ColorMagenta+"分数:"+ColorReset+" "+scoreColor+"%d"+ColorReset+"/100\n", candidate.Score)

		// 更新最佳邮箱
		if candidate.Score > bestScore {
			bestEmail = candidate.Email
			bestScore = candidate.Score
		}
	}

	fmt.Println()

	// 如果没有成功生成任何邮箱
	if len(candidates) == 0 {
		return nil, fmt.Errorf("所有生成尝试均失败")
	}

	// 如果启用自动选择且有满足条件的邮箱
	if qualityConfig.AutoSelect && bestScore >= qualityConfig.MinScore {
		fmt.Printf("  " + ColorBrightGreen + "[+] 自动选择最佳邮箱 (分数: %d)" + ColorReset + "\n\n", bestScore)

		// 确认创建邮箱
		finalEmail, err := reserveHME(config, bestEmail, label)
		if err != nil {
			return nil, fmt.Errorf("确认创建邮箱失败: %v", err)
		}

		return &EmailQualityResult{
			Candidates:   candidates,
			BestEmail:    finalEmail,
			BestScore:    bestScore,
			TotalTries:   len(candidates),
			AutoSelected: true,
		}, nil
	}

	// 返回所有候选项供手动选择
	return &EmailQualityResult{
		Candidates:   candidates,
		BestEmail:    bestEmail,
		BestScore:    bestScore,
		TotalTries:   len(candidates),
		AutoSelected: false,
	}, nil
}

// 手动选择邮箱
func selectEmailManually(result *EmailQualityResult, config *Config, label string) (string, error) {
	if len(result.Candidates) == 0 {
		return "", fmt.Errorf("没有可选择的邮箱")
	}

	printSubHeader("邮箱选择")
	fmt.Printf("  "+ColorBold+"共生成 %d 个邮箱"+ColorReset+" "+ColorDim+"(推荐: ID%d)"+ColorReset+"\n\n", len(result.Candidates), getBestCandidateID(result.Candidates))

	// 显示所有候选邮箱
	for _, candidate := range result.Candidates {
		var scoreColor, statusIcon string
		if candidate.Score >= config.EmailQuality.MinScore {
			scoreColor = ColorGreen
			statusIcon = ColorGreen + "[+]" + ColorReset
		} else if candidate.Score >= config.EmailQuality.MinScore-20 {
			scoreColor = ColorYellow
			statusIcon = ColorYellow + "[~]" + ColorReset
		} else {
			scoreColor = ColorRed
			statusIcon = ColorRed + "[!]" + ColorReset
		}

		fmt.Printf("  "+ColorBrightCyan+"ID%d."+ColorReset+" %s "+ColorBrightWhite+"%s"+ColorReset+"\n",
			candidate.ID, statusIcon, candidate.Email)
		fmt.Printf("      "+ColorMagenta+"分数:"+ColorReset+" "+scoreColor+"%d"+ColorReset+"/100", candidate.Score)

		if candidate.Email == result.BestEmail {
			fmt.Printf(" " + ColorBold + ColorBrightGreen + "(最佳)" + ColorReset)
		}
		fmt.Println()

		// 显示详细评分
		if config.EmailQuality.ShowScores {
			showDetailedScore(candidate.Email, config.EmailQuality.Weights)
		}
		fmt.Println()
	}

	// 用户选择
	printInfo("输入 ID 选择邮箱 (1-3)，或输入 'auto' 自动选择最佳")
	input := readInput("选择: ")
	input = strings.TrimSpace(strings.ToLower(input))

	var selectedEmail string
	if input == "auto" || input == "" {
		selectedEmail = result.BestEmail
		fmt.Printf("\n  "+ColorBrightGreen+"[+] 自动选择最佳邮箱"+ColorReset+" (分数: %d)\n", result.BestScore)
	} else {
		id, err := strconv.Atoi(input)
		if err != nil || id < 1 || id > len(result.Candidates) {
			return "", fmt.Errorf("无效的选择: %s", input)
		}

		// 找到对应ID的邮箱
		for _, candidate := range result.Candidates {
			if candidate.ID == id {
				selectedEmail = candidate.Email
				fmt.Printf("\n  "+ColorBrightGreen+"[+] 已选择 ID%d"+ColorReset+" (分数: %d)\n", id, candidate.Score)
				break
			}
		}

		if selectedEmail == "" {
			return "", fmt.Errorf("找不到 ID%d 对应的邮箱", id)
		}
	}

	// 确认创建邮箱
	fmt.Printf("\n  " + ColorDim + "..." + ColorReset + " 确认创建邮箱 ... ")
	finalEmail, err := reserveHME(config, selectedEmail, label)
	if err != nil {
		fmt.Printf(ColorRed + "[!]" + ColorReset + "\n")
		return "", fmt.Errorf("确认创建邮箱失败: %v", err)
	}
	fmt.Printf(ColorGreen + "[+]" + ColorReset + "\n")

	return finalEmail, nil
}

// 获取最佳候选邮箱的ID
func getBestCandidateID(candidates []EmailCandidate) int {
	if len(candidates) == 0 {
		return 0
	}

	bestScore := -1
	bestID := 0
	for _, candidate := range candidates {
		if candidate.Score > bestScore {
			bestScore = candidate.Score
			bestID = candidate.ID
		}
	}
	return bestID
}

// 显示详细评分
func showDetailedScore(email string, weights ScoreWeights) {
	if email == "" {
		return
	}

	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return
	}
	prefix := parts[0]
	domain := parts[1]

	fmt.Printf("      " + ColorDim + "详细评分:" + ColorReset)

	if weights.PrefixStructure > 0 {
		score := evaluatePrefixStructure(prefix)
		fmt.Printf(" "+ColorCyan+"结构"+ColorReset+":%d", score)
	}

	if weights.Length > 0 {
		score := evaluateLength(prefix)
		fmt.Printf(" "+ColorBlue+"长度"+ColorReset+":%d", score)
	}

	if weights.Readability > 0 {
		score := evaluateReadability(prefix)
		fmt.Printf(" "+ColorYellow+"可读"+ColorReset+":%d", score)
	}

	if weights.Security > 0 {
		score := evaluateSecurity(prefix, domain)
		fmt.Printf(" "+ColorMagenta+"安全"+ColorReset+":%d", score)
	}
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
	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
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

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
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

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
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

	req, err := http.NewRequest("POST", url, bytes.NewReader(jsonData))
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

	printSubHeader("批量创建执行中")

	// 确定并发数
	concurrency := config.MaxConcurrency
	if concurrency <= 0 {
		concurrency = 1 // 默认串行
	} else if concurrency > count {
		concurrency = count
	}

	fmt.Printf("  "+ColorCyan+"数量:"+ColorReset+" %d "+ColorDim+"|"+ColorReset+" "+ColorCyan+"标签:"+ColorReset+" %s* "+ColorDim+"|"+ColorReset+" "+ColorCyan+"并发:"+ColorReset+" %d\n\n", count, labelPrefix, concurrency)

	// 使用并发模式
	if concurrency > 1 {
		return batchGenerateConcurrent(config, count, labelPrefix, concurrency)
	}

	// 串行模式（原有逻辑）
	emails := make([]string, 0, count)
	errs := make([]error, 0, count)

	for i := 0; i < count; i++ {
		label := fmt.Sprintf("%s%d", labelPrefix, i+1)

		// 显示进度条
		printProgressBar(i, count, "创建进度")

		fmt.Printf("  "+ColorGray+"..."+ColorReset+" 创建邮箱 "+ColorDim+"(%s)"+ColorReset+" ... ", label)

		email, err := createHME(config, label)
		if err != nil {
			fmt.Printf(ColorRed + "[!]" + ColorReset + "\n")
			fmt.Printf("    错误: %v\n", err)
			errs = append(errs, err)
		} else {
			fmt.Printf(ColorGreen + "[+]" + ColorReset + "\n")
			fmt.Printf("    "+ColorCyan+"邮箱:"+ColorReset+" %s\n", email)
			emails = append(emails, email)

			// 保存邮箱到文件
			if err := saveEmailToFile(config, email, label); err != nil {
				fmt.Printf("    "+ColorYellow+"警告:"+ColorReset+" 保存到文件失败: %v\n", err)
			}
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

// 并发批量生成邮箱
func batchGenerateConcurrent(config *Config, count int, labelPrefix string, concurrency int) ([]string, []error) {
	// 结果通道
	type result struct {
		index int
		email string
		label string
		err   error
	}

	resultChan := make(chan result, count)
	semaphore := make(chan struct{}, concurrency) // 并发控制

	var wg sync.WaitGroup
	var progressMutex sync.Mutex
	completed := 0

	// 启动并发任务
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// 获取信号量
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			label := fmt.Sprintf("%s%d", labelPrefix, index+1)
			email, err := createHME(config, label)

			// 发送结果
			resultChan <- result{
				index: index,
				email: email,
				label: label,
				err:   err,
			}

			// 更新进度
			progressMutex.Lock()
			completed++
			printProgressBar(completed, count, "创建进度")
			progressMutex.Unlock()

			// 延迟（避免请求过快）
			if config.DelaySeconds > 0 {
				time.Sleep(time.Duration(config.DelaySeconds) * time.Second)
			}
		}(i)
	}

	// 等待所有任务完成
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// 收集结果
	results := make([]result, 0, count)
	for r := range resultChan {
		results = append(results, r)
	}

	// 按索引排序结果
	sortedResults := make([]result, count)
	for _, r := range results {
		sortedResults[r.index] = r
	}

	// 提取邮箱和错误
	emails := make([]string, 0, count)
	errs := make([]error, 0)

	fmt.Println() // 换行
	for _, r := range sortedResults {
		if r.err != nil {
			fmt.Printf("  "+ColorRed+"[!]"+ColorReset+" %s: %v\n", r.label, r.err)
			errs = append(errs, r.err)
		} else {
			fmt.Printf("  "+ColorGreen+"[+]"+ColorReset+" %s: %s\n", r.label, r.email)
			emails = append(emails, r.email)

			// 保存邮箱到文件
			if err := saveEmailToFile(config, r.email, r.label); err != nil {
				fmt.Printf("    "+ColorYellow+"警告:"+ColorReset+" 保存到文件失败: %v\n", err)
			}
		}
	}

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

// clearScreen 清屏函数
func clearScreen() {
	fmt.Print("\033[2J\033[H")
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
	fmt.Printf(ColorGreen+"  [+]"+ColorReset+" %s\n", message)
}

func printError(message string) {
	fmt.Printf(ColorRed+"  [!]"+ColorReset+" %s\n", message)
}

func printWarning(message string) {
	fmt.Printf(ColorYellow+"  !"+ColorReset+" %s\n", message)
}

func printInfo(message string) {
	fmt.Printf("  "+ColorCyan+"›"+ColorReset+" %s\n", message)
}

func printStep(message string) {
	fmt.Printf("  "+ColorDim+"..."+ColorReset+" %s\n", message)
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
		statusSymbol := "[+]"
		statusText := ColorGreen + "完成" + ColorReset
		if err != nil {
			statusColor = ColorBrightRed
			statusSymbol = "[!]"
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
	fmt.Println("  " + ColorBlue + "[2]" + ColorReset + " 创建新邮箱 " + ColorDim + "(普通模式)" + ColorReset)
	fmt.Println("  " + ColorBrightBlue + "[3]" + ColorReset + " 智能创建邮箱 " + ColorBrightGreen + "(推荐)" + ColorReset)
	fmt.Println("  " + ColorYellow + "[4]" + ColorReset + " 停用邮箱")
	fmt.Println("  " + ColorMagenta + "[5]" + ColorReset + " 批量创建邮箱")
	fmt.Println("  " + ColorRed + "[6]" + ColorReset + " 彻底删除停用的邮箱 " + ColorDim + "(不可恢复)" + ColorReset)
	fmt.Println("  " + ColorCyan + "[7]" + ColorReset + " 重新激活停用的邮箱")
	fmt.Println("  " + ColorBrightMagenta + "[8]" + ColorReset + " 程序设置")

	// 开发者模式下显示测试选项
	config := getCurrentConfig()
	if config != nil && config.DeveloperMode {
		fmt.Println("  " + ColorGray + "[9]" + ColorReset + " 测试评分算法 " + ColorDim + "(开发调试)" + ColorReset)
	}
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
		fmt.Printf("      "+ColorBrightBlue+"# 标签: "+ColorReset+ColorCyan+"%s"+ColorReset+"\n", email.Label)

		if email.ForwardToEmail != "" {
			fmt.Printf("      "+ColorBrightMagenta+"➤ 转发: "+ColorReset+ColorMagenta+"%s"+ColorReset+"\n", email.ForwardToEmail)
		}

		// 显示创建时间
		createTime := time.Unix(email.CreateTimestamp/1000, 0)
		fmt.Printf("      "+ColorBrightGreen+"& 创建: "+ColorReset+ColorGreen+"%s"+ColorReset+"\n", createTime.Format("2006-01-02 15:04"))
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

	// 保存邮箱到文件
	if err := saveEmailToFile(config, email, label); err != nil {
		printWarning(fmt.Sprintf("保存邮箱到文件失败: %v", err))
	}

	fmt.Println()
	printSuccess("邮箱创建成功")
	fmt.Printf("\n  "+ColorBrightMagenta+"@ 邮箱: "+ColorReset+ColorBold+ColorBrightWhite+"%s"+ColorReset+"\n", email)
	fmt.Printf("  "+ColorBrightBlue+"# 标签: "+ColorReset+ColorCyan+"%s"+ColorReset+"\n", label)
	fmt.Printf("  "+ColorBrightGreen+"& 时间: "+ColorReset+ColorGreen+"%s"+ColorReset+"\n", time.Now().Format("2006-01-02 15:04"))
}

// 智能创建邮箱
func handleSmartCreateEmail(config *Config) {
	printHeader("智能创建邮箱")

	// 显示当前设置
	fmt.Printf("  " + ColorBold + "当前设置" + ColorReset + "\n\n")
	fmt.Printf("  "+ColorCyan+"自动选择:"+ColorReset+" %v\n", config.EmailQuality.AutoSelect)
	fmt.Printf("  "+ColorCyan+"最低分数:"+ColorReset+" %d/100\n", config.EmailQuality.MinScore)
	fmt.Printf("  "+ColorCyan+"最大尝试:"+ColorReset+" %d 次\n", config.EmailQuality.MaxRegenerateCount)
	fmt.Printf("  "+ColorCyan+"显示详分:"+ColorReset+" %v\n", config.EmailQuality.ShowScores)
	fmt.Println()

	label := readInput("邮箱标签: ")
	if label == "" {
		printError("标签不能为空")
		return
	}

	// 生成智能邮箱
	result, err := generateSmartEmail(config, label)
	if err != nil {
		printError(fmt.Sprintf("智能生成失败: %v", err))
		return
	}

	var finalEmail string
	if result.AutoSelected {
		// 已自动选择
		finalEmail = result.BestEmail
		printSuccess("邮箱创建成功 (自动选择)")
	} else {
		// 需要手动选择
		if config.EmailQuality.AllowManual {
			finalEmail, err = selectEmailManually(result, config, label)
			if err != nil {
				printError(fmt.Sprintf("手动选择失败: %v", err))
				return
			}
			printSuccess("邮箱创建成功 (手动选择)")
		} else {
			// 自动选择最佳
			finalEmail, err = reserveHME(config, result.BestEmail, label)
			if err != nil {
				printError(fmt.Sprintf("确认创建失败: %v", err))
				return
			}
			printSuccess("邮箱创建成功 (自动选择最佳)")
		}
	}

	// 保存邮箱到文件
	if err := saveEmailToFile(config, finalEmail, label); err != nil {
		printWarning(fmt.Sprintf("保存邮箱到文件失败: %v", err))
	}

	// 显示最终结果
	fmt.Println()
	fmt.Printf("  "+ColorBrightMagenta+"@ 邮箱: "+ColorReset+ColorBold+ColorBrightWhite+"%s"+ColorReset+"\n", finalEmail)
	fmt.Printf("  "+ColorBrightBlue+"# 标签: "+ColorReset+ColorCyan+"%s"+ColorReset+"\n", label)
	fmt.Printf("  "+ColorBrightGreen+"* 分数: "+ColorReset+ColorGreen+"%d"+ColorReset+"/100\n", result.BestScore)
	fmt.Printf("  "+ColorBrightYellow+"~ 尝试: "+ColorReset+ColorYellow+"%d"+ColorReset+" 次\n", result.TotalTries)
	fmt.Printf("  "+ColorBrightGreen+"& 时间: "+ColorReset+ColorGreen+"%s"+ColorReset+"\n", time.Now().Format("2006-01-02 15:04"))
}

// 程序设置
func handleProgramSettings(config *Config) {
	for {
		printHeader("程序设置")

		fmt.Printf("  " + ColorBold + "当前配置" + ColorReset + "\n\n")
		fmt.Printf("  " + ColorGreen + "[1]" + ColorReset + " 邮箱质量设置\n")
		fmt.Printf("  " + ColorBlue + "[2]" + ColorReset + " 邮箱保存设置\n")
		fmt.Printf("  "+ColorYellow+"[3]"+ColorReset+" 开发者模式: %s\n", formatBoolSetting(config.DeveloperMode))
		fmt.Printf("  " + ColorDim + "[0]" + ColorReset + " 返回主菜单\n")

		printSeparator()
		fmt.Println()

		choice := readInput("选择设置项 (0-3): ")
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			handleEmailQualitySettings(config)
		case "2":
			handleEmailSaveSettings(config)
		case "3":
			config.DeveloperMode = !config.DeveloperMode
			saveConfigWithMessage(config, fmt.Sprintf("开发者模式已设置为: %v", config.DeveloperMode))
		case "0":
			return
		default:
			printError("无效选择，请输入 0-3")
		}

		fmt.Print("\n  " + ColorDim + "按回车键继续..." + ColorReset)
		readInput("")
	}
}

// 邮箱质量设置
func handleEmailQualitySettings(config *Config) {
	for {
		printHeader("邮箱质量设置")

		fmt.Printf("  " + ColorBold + "当前配置" + ColorReset + "\n\n")
		fmt.Printf("  "+ColorGreen+"[1]"+ColorReset+" 自动选择: %s\n", formatBoolSetting(config.EmailQuality.AutoSelect))
		fmt.Printf("  "+ColorBlue+"[2]"+ColorReset+" 最低分数: "+ColorCyan+"%d"+ColorReset+"/100\n", config.EmailQuality.MinScore)
		fmt.Printf("  "+ColorYellow+"[3]"+ColorReset+" 最大尝试: "+ColorCyan+"%d"+ColorReset+" 次\n", config.EmailQuality.MaxRegenerateCount)
		fmt.Printf("  "+ColorMagenta+"[4]"+ColorReset+" 显示详分: %s\n", formatBoolSetting(config.EmailQuality.ShowScores))
		fmt.Printf("  "+ColorCyan+"[5]"+ColorReset+" 允许手动: %s\n", formatBoolSetting(config.EmailQuality.AllowManual))
		fmt.Printf("  " + ColorBrightBlue + "[6]" + ColorReset + " 评分权重设置\n")
		fmt.Printf("  " + ColorBrightGreen + "[7]" + ColorReset + " 重置为默认值\n")
		fmt.Printf("  " + ColorBrightYellow + "[8]" + ColorReset + " 邮箱保存设置\n")
		fmt.Printf("  " + ColorDim + "[0]" + ColorReset + " 返回主菜单\n")

		printSeparator()
		fmt.Println()

		choice := readInput("选择设置项 (0-8): ")
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			config.EmailQuality.AutoSelect = !config.EmailQuality.AutoSelect
			saveConfigWithMessage(config, fmt.Sprintf("自动选择已设置为: %v", config.EmailQuality.AutoSelect))
		case "2":
			score, err := readInt("输入最低分数 (0-100): ")
			if err != nil || score < 0 || score > 100 {
				printError("请输入 0-100 之间的数字")
			} else {
				config.EmailQuality.MinScore = score
				saveConfigWithMessage(config, fmt.Sprintf("最低分数已设置为: %d", score))
			}
		case "3":
			tries, err := readInt("输入最大尝试次数 (1-5): ")
			if err != nil || tries < 1 || tries > 5 {
				printError("请输入 1-5 之间的数字")
			} else {
				config.EmailQuality.MaxRegenerateCount = tries
				saveConfigWithMessage(config, fmt.Sprintf("最大尝试次数已设置为: %d", tries))
			}
		case "4":
			config.EmailQuality.ShowScores = !config.EmailQuality.ShowScores
			saveConfigWithMessage(config, fmt.Sprintf("显示详细评分已设置为: %v", config.EmailQuality.ShowScores))
		case "5":
			config.EmailQuality.AllowManual = !config.EmailQuality.AllowManual
			saveConfigWithMessage(config, fmt.Sprintf("允许手动选择已设置为: %v", config.EmailQuality.AllowManual))
		case "6":
			handleWeightSettings(config)
		case "7":
			resetToDefaults(config)
			saveConfigWithMessage(config, "已重置为默认设置")
		case "8":
			handleEmailSaveSettings(config)
		case "0":
			return
		default:
			printError("无效选择，请输入 0-8")
		}

		fmt.Print("\n  " + ColorDim + "按回车键继续..." + ColorReset)
		readInput("")
	}
}

// 格式化布尔设置显示
func formatBoolSetting(value bool) string {
	if value {
		return ColorGreen + "启用" + ColorReset
	}
	return ColorRed + "禁用" + ColorReset
}

// 保存邮箱到文件
func saveEmailToFile(config *Config, email, label string) error {
	if !config.SaveGeneratedEmails {
		return nil // 如果未启用保存功能，直接返回
	}

	// 创建邮箱记录
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	record := fmt.Sprintf("[%s] @ 邮箱: %s | # 标签: %s\n", timestamp, email, label)

	// 追加到文件
	file, err := os.OpenFile(config.EmailListFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("无法打开邮箱保存文件: %v", err)
	}
	defer file.Close()

	if _, err := file.WriteString(record); err != nil {
		return fmt.Errorf("无法写入邮箱记录: %v", err)
	}

	return nil
}

// 邮箱保存设置
func handleEmailSaveSettings(config *Config) {
	for {
		printHeader("邮箱保存设置")

		fmt.Printf("  " + ColorBold + "当前配置" + ColorReset + "\n\n")
		fmt.Printf("  "+ColorGreen+"[1]"+ColorReset+" 保存生成的邮箱: %s\n", formatBoolSetting(config.SaveGeneratedEmails))
		fmt.Printf("  "+ColorBlue+"[2]"+ColorReset+" 保存文件路径: "+ColorCyan+"%s"+ColorReset+"\n", config.EmailListFile)
		fmt.Printf("  " + ColorDim + "[0]" + ColorReset + " 返回上级菜单\n")

		printSeparator()
		fmt.Println()

		choice := readInput("选择设置项 (0-2): ")
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			config.SaveGeneratedEmails = !config.SaveGeneratedEmails
			saveConfigWithMessage(config, fmt.Sprintf("保存生成的邮箱已设置为: %v", config.SaveGeneratedEmails))
		case "2":
			filename := readInput("输入保存文件名: ")
			filename = strings.TrimSpace(filename)
			if filename != "" {
				config.EmailListFile = filename
				saveConfigWithMessage(config, fmt.Sprintf("保存文件路径已设置为: %s", filename))
			} else {
				printError("文件名不能为空")
			}
		case "0":
			return
		default:
			printError("无效选择，请输入 0-2")
		}

		fmt.Print("\n  " + ColorDim + "按回车键继续..." + ColorReset)
		readInput("")
	}
}

// 权重设置
func handleWeightSettings(config *Config) {
	for {
		printHeader("评分权重设置")

		weights := &config.EmailQuality.Weights
		total := weights.PrefixStructure + weights.Length + weights.Readability + weights.Security

		fmt.Printf("  "+ColorBold+"当前权重配置"+ColorReset+" "+ColorDim+"(总计: %d)"+ColorReset+"\n\n", total)
		fmt.Printf("  "+ColorGreen+"[1]"+ColorReset+" 前缀结构: "+ColorCyan+"%d"+ColorReset+"\n", weights.PrefixStructure)
		fmt.Printf("  "+ColorBlue+"[2]"+ColorReset+" 长度评分: "+ColorCyan+"%d"+ColorReset+"\n", weights.Length)
		fmt.Printf("  "+ColorYellow+"[3]"+ColorReset+" 可读性评分: "+ColorCyan+"%d"+ColorReset+"\n", weights.Readability)
		fmt.Printf("  "+ColorMagenta+"[4]"+ColorReset+" 安全性评分: "+ColorCyan+"%d"+ColorReset+"\n", weights.Security)
		fmt.Printf("  " + ColorBrightGreen + "[5]" + ColorReset + " 重置为推荐值\n")
		fmt.Printf("  " + ColorDim + "[0]" + ColorReset + " 返回上级菜单\n")

		printSeparator()
		fmt.Println()

		choice := readInput("选择权重项 (0-5): ")
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			weight, err := readInt("输入前缀结构权重 (0-100): ")
			if err != nil || weight < 0 || weight > 100 {
				printError("请输入 0-100 之间的数字")
			} else {
				weights.PrefixStructure = weight
				saveConfigWithMessage(config, fmt.Sprintf("前缀结构权重已设置为: %d", weight))
			}
		case "2":
			weight, err := readInt("输入长度权重 (0-100): ")
			if err != nil || weight < 0 || weight > 100 {
				printError("请输入 0-100 之间的数字")
			} else {
				weights.Length = weight
				saveConfigWithMessage(config, fmt.Sprintf("长度权重已设置为: %d", weight))
			}
		case "3":
			weight, err := readInt("输入可读性权重 (0-100): ")
			if err != nil || weight < 0 || weight > 100 {
				printError("请输入 0-100 之间的数字")
			} else {
				weights.Readability = weight
				saveConfigWithMessage(config, fmt.Sprintf("可读性权重已设置为: %d", weight))
			}
		case "4":
			weight, err := readInt("输入安全性权重 (0-100): ")
			if err != nil || weight < 0 || weight > 100 {
				printError("请输入 0-100 之间的数字")
			} else {
				weights.Security = weight
				saveConfigWithMessage(config, fmt.Sprintf("安全性权重已设置为: %d", weight))
			}
		case "5":
			// 推荐权重配置
			weights.PrefixStructure = 40
			weights.Length = 20
			weights.Readability = 25
			weights.Security = 15
			saveConfigWithMessage(config, "已重置为推荐权重配置")
		case "0":
			return
		default:
			printError("无效选择，请输入 0-5")
		}

		fmt.Print("\n  " + ColorDim + "按回车键继续..." + ColorReset)
		readInput("")
	}
}

// 重置为默认设置
func resetToDefaults(config *Config) {
	config.EmailQuality = EmailQualityConfig{
		AutoSelect:         false,
		MinScore:           70,
		MaxRegenerateCount: 3,
		ShowScores:         true,
		AllowManual:        true,
		ShowAllEmails:      true,
		Weights: ScoreWeights{
			PrefixStructure: 40,
			Length:          20,
			Readability:     25,
			Security:        15,
		},
	}
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
		fmt.Printf("  "+ColorDim+"..."+ColorReset+" 停用 %s ... ", email.HME)

		err := deactivateHME(config, email.AnonymousID)
		if err != nil {
			fmt.Printf(ColorRed + "[!]" + ColorReset + "\n")
			fmt.Printf("    错误: %v\n", err)
			failCount++
		} else {
			fmt.Printf(ColorGreen + "[+]" + ColorReset + "\n")
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
			fmt.Printf("  "+ColorDim+"%2d."+ColorReset+" "+ColorGreen+"[+]"+ColorReset+" %s\n", i+1, email)
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
		fmt.Printf("  "+ColorDim+"..."+ColorReset+" 删除 %s ... ", email.HME)

		err := permanentDeleteHME(config, email.AnonymousID)
		if err != nil {
			fmt.Printf(ColorRed + "[!]" + ColorReset + "\n")
			fmt.Printf("    错误: %v\n", err)
			failCount++
		} else {
			fmt.Printf(ColorGreen + "[+]" + ColorReset + "\n")
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
		fmt.Printf("  "+ColorDim+"..."+ColorReset+" 激活 %s ... ", email.HME)

		err := reactivateHME(config, email.AnonymousID)
		if err != nil {
			fmt.Printf(ColorRed + "[!]" + ColorReset + "\n")
			fmt.Printf("    错误: %v\n", err)
			failCount++
		} else {
			fmt.Printf(ColorGreen + "[+]" + ColorReset + "\n")
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

// 测试邮箱评分算法
func testEmailScoring() {
	printHeader("邮箱评分算法测试")

	// 测试权重配置
	weights := ScoreWeights{
		PrefixStructure: 40,
		Length:          20,
		Readability:     25,
		Security:        15,
	}

	// 测试邮箱列表
	testEmails := []string{
		"john.smith@icloud.com",                       // 理想邮箱
		"johnsmith@icloud.com",                        // 纯字母
		"john123@icloud.com",                          // 字母+数字
		"a3x9kf@icloud.com",                           // 随机字符
		"test_temp@icloud.com",                        // 临时邮箱特征
		"kettles.doltish_8p@icloud.com",               // 实际生成的例子
		"user@gmail.com",                              // Gmail域名
		"verylongusernamethatexceedslimit@icloud.com", // 过长
		"ab@icloud.com",                               // 过短
		"mike.work.2024@icloud.com",                   // 复杂结构
	}

	fmt.Printf("  "+ColorBold+"权重配置"+ColorReset+": 结构(%d) 长度(%d) 可读(%d) 安全(%d)\n\n",
		weights.PrefixStructure, weights.Length, weights.Readability, weights.Security)

	for i, email := range testEmails {
		score := evaluateEmailQuality(email, weights)

		// 分离前缀和域名用于详细分析
		parts := strings.Split(email, "@")
		prefix := parts[0]
		domain := parts[1]

		// 计算各项分数
		structureScore := evaluatePrefixStructure(prefix)
		lengthScore := evaluateLength(prefix)
		readabilityScore := evaluateReadability(prefix)
		securityScore := evaluateSecurity(prefix, domain)

		// 评级和颜色
		var grade, gradeColor string
		if score >= 85 {
			grade = "优秀"
			gradeColor = ColorBrightGreen
		} else if score >= 70 {
			grade = "良好"
			gradeColor = ColorGreen
		} else if score >= 60 {
			grade = "一般"
			gradeColor = ColorYellow
		} else {
			grade = "较差"
			gradeColor = ColorRed
		}

		fmt.Printf("  "+ColorBrightCyan+"%2d."+ColorReset+" %s\n", i+1, email)
		fmt.Printf("      "+ColorMagenta+"总分:"+ColorReset+" "+gradeColor+"%d"+ColorReset+"/100 "+ColorDim+"("+gradeColor+"%s"+ColorReset+ColorDim+")"+ColorReset+"\n", score, grade)
		fmt.Printf("      "+ColorDim+"详细:"+ColorReset+" 结构(%d) 长度(%d) 可读(%d) 安全(%d)\n\n",
			structureScore, lengthScore, readabilityScore, securityScore)
	}

	printSubHeader("评分标准说明")
	fmt.Println("  " + ColorBrightGreen + "85+ 分: 优秀" + ColorReset + " - 适合重要账户注册")
	fmt.Println("  " + ColorGreen + "70+ 分: 良好" + ColorReset + " - 适合一般用途")
	fmt.Println("  " + ColorYellow + "60+ 分: 一般" + ColorReset + " - 可接受但不推荐")
	fmt.Println("  " + ColorRed + "60- 分: 较差" + ColorReset + " - 建议重新生成")
}

// 初始化管理器
func initializeManagers() {
	// 初始化配置管理器
	configManager = NewConfigManager(CONFIG_FILE)

	// 初始化进程安全管理器
	safetyManager = NewProcessSafetyManager()

	// 初始化网络管理器 (默认30秒超时，3次重试)
	networkManager = NewNetworkManager(30*time.Second, 3)
}

// 设置信号处理
func setupSignalHandlers() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\n\n" + ColorYellow + "[!] 接收到退出信号，正在安全退出..." + ColorReset)

		// 释放进程锁
		if safetyManager != nil {
			safetyManager.Unlock()
		}

		// 清理锁文件
		os.Remove(LOCK_FILE)

		fmt.Println(ColorGreen + "[+] 程序已安全退出" + ColorReset)
		os.Exit(0)
	}()
}

// 启动配置热重载监控（使用 fsnotify 优化）
func startConfigWatcher() {
	go func() {
		// 创建文件监控器
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			fmt.Printf(ColorYellow+"[!] 无法启动配置文件监控: %v"+ColorReset+"\n", err)
			return
		}
		defer watcher.Close()

		// 监听当前目录而非文件，以支持 vim/VS Code 等编辑器的原子写操作
		err = watcher.Add(".")
		if err != nil {
			fmt.Printf(ColorYellow+"[!] 无法监控当前目录: %v"+ColorReset+"\n", err)
			return
		}

		var reloadAttempts int
		const maxReloadAttempts = 3

		// 使用 debounce 避免多次触发
		var debounceTimer *time.Timer
		const debounceDelay = 500 * time.Millisecond

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// 只处理 config.json 的写入、创建和重命名事件
				if event.Name != CONFIG_FILE && event.Name != "./"+CONFIG_FILE {
					continue
				}

				// 处理写入、创建和重命名事件（支持编辑器的原子写操作）
				if event.Op&fsnotify.Write == fsnotify.Write ||
					event.Op&fsnotify.Create == fsnotify.Create ||
					event.Op&fsnotify.Rename == fsnotify.Rename {

					// Debounce: 延迟处理，避免多次快速写入
					if debounceTimer != nil {
						debounceTimer.Stop()
					}

					debounceTimer = time.AfterFunc(debounceDelay, func() {
						// 检查文件是否存在（处理重命名情况）
						if _, err := os.Stat(CONFIG_FILE); os.IsNotExist(err) {
							return
						}

						fmt.Printf("\n" + ColorYellow + "[!] 检测到配置文件更新，正在重新加载..." + ColorReset + "\n")

						newConfig, err := configManager.LoadConfig()
						if err != nil {
							reloadAttempts++
							fmt.Printf(ColorRed+"[!] 重新加载配置失败: %v"+ColorReset+"\n", err)

							if reloadAttempts >= maxReloadAttempts {
								fmt.Printf(ColorRed+"[!] 配置重载失败次数过多 (%d/%d)"+ColorReset+"\n", reloadAttempts, maxReloadAttempts)
								fmt.Printf(ColorYellow + "[!] 修复建议:" + ColorReset + "\n")
								fmt.Printf("  1. 检查 config.json 文件格式是否正确\n")
								fmt.Printf("  2. 确保 JSON 语法无误\n")
								fmt.Printf("  3. 恢复备份的配置文件\n")
								fmt.Printf("  4. 重启程序\n")
								fmt.Printf(ColorRed + "[!] 程序将安全退出..." + ColorReset + "\n")

								// 安全退出
								if safetyManager != nil {
									safetyManager.Unlock()
								}
								os.Remove(LOCK_FILE)
								fmt.Println(ColorGreen + "[+] 程序已安全退出" + ColorReset)
								os.Exit(1)
								return
							}
							return
						}

						// 重置重试计数
						reloadAttempts = 0

						// 更新全局配置
						configMutex.Lock()
						globalConfig = newConfig
						configMutex.Unlock()

						// 清屏并重新显示主菜单
						clearScreen()
						fmt.Printf(ColorGreen + "[+] 配置已成功重新加载" + ColorReset + "\n")
						showMainMenu()
					})
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Printf(ColorYellow+"[!] 配置文件监控错误: %v"+ColorReset+"\n", err)

			case <-safetyManager.Context().Done():
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				return
			}
		}
	}()
}

// 获取当前配置 (线程安全)
func getCurrentConfig() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// 保存配置并显示消息
func saveConfigWithMessage(config *Config, message string) {
	// 更新全局配置
	configMutex.Lock()
	globalConfig = config
	configMutex.Unlock()

	// 保存到文件
	if err := configManager.SaveConfig(config); err != nil {
		printError(fmt.Sprintf("保存配置失败: %v", err))
	} else {
		printSuccess(message + " (已保存)")
	}
}

func main() {
	// 初始化管理器
	initializeManagers()

	// 设置信号处理
	setupSignalHandlers()

	// 获取进程锁
	if err := safetyManager.Lock(); err != nil {
		printError(fmt.Sprintf("启动失败: %v", err))
		os.Exit(1)
	}
	defer safetyManager.Unlock()

	// 显示启动信息
	printHeader("iCloud 隐藏邮箱管理工具")
	fmt.Printf("  " + ColorCyan + "版本:" + ColorReset + " " + ColorBold + VERSION + ColorReset + "\n")
	fmt.Printf("  " + ColorCyan + "作者:" + ColorReset + " " + AUTHOR + "\n")
	fmt.Println()

	// 加载配置
	var config *Config
	if err := withSpinner("加载配置文件", func() error {
		var err error
		config, err = configManager.LoadConfig()
		if err != nil {
			return err
		}

		// 设置全局配置
		configMutex.Lock()
		globalConfig = config
		configMutex.Unlock()

		return nil
	}); err != nil {
		printError(fmt.Sprintf("加载失败: %v", err))
		printInfo("请确保 config.json 文件存在且格式正确")
		os.Exit(1)
	}

	// 启动配置热重载监控
	startConfigWatcher()

	// 主循环
	for {
		showMainMenu()
		choice := readInput("选择操作 (0-9): ")
		choice = strings.ToLower(strings.TrimSpace(choice))

		switch choice {
		case "1", "l", "list":
			handleListEmails(config)
		case "2", "c", "create":
			handleCreateEmail(config)
		case "3", "s", "smart":
			handleSmartCreateEmail(config)
		case "4", "d", "deactivate":
			handleDeleteEmails(config)
		case "5", "b", "batch":
			handleBatchCreate(config)
		case "6", "delete":
			handlePermanentDelete(config)
		case "7", "r", "reactivate":
			handleReactivate(config)
		case "8", "settings":
			handleProgramSettings(config)
		case "9", "test":
			if config.DeveloperMode {
				testEmailScoring()
			} else {
				printError("无效选择，请输入 0-8")
			}
		case "0", "q", "quit", "exit", "e":
			fmt.Println()
			printThickSeparator()
			fmt.Printf("  感谢使用\n")
			printThickSeparator()
			return
		default:
			printError("无效选择，请输入 0-9 或对应字母")
		}

		fmt.Print("\n  " + ColorDim + "按回车键继续..." + ColorReset)
		readInput("")

		// 清屏效果
		fmt.Print("\033[2J\033[H")
	}
}
