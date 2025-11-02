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
	generateURL := strings.Replace(config.BaseURL, "/reserve", "/generate", 1)
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

	// 设置请求头
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应（处理 gzip 压缩）
	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("无法创建 gzip reader: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("无法读取响应: %v", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var response GenerateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("无法解析响应: %v, 原始响应: %s", err, string(body))
	}

	// 检查是否成功
	if !response.Success {
		return "", fmt.Errorf("API返回失败: %s", string(body))
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

	// 设置请求头
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应（处理 gzip 压缩）
	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return "", fmt.Errorf("无法创建 gzip reader: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("无法读取响应: %v", err)
	}

	// 检查HTTP状态码
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var response ReserveResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("无法解析响应: %v, 原始响应: %s", err, string(body))
	}

	// 检查是否成功
	if !response.Success {
		return "", fmt.Errorf("API返回失败: %s", string(body))
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
	listURL := strings.Replace(config.BaseURL, "/v1/hme/reserve", "/v2/hme/list", 1)
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

	// 设置请求头
	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	// 发送请求
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("解压响应失败: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("服务器返回错误 (状态码: %d)", resp.StatusCode)
	}

	var response ListResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析响应失败: %v", err)
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
	deactivateURL := strings.Replace(config.BaseURL, "/reserve", "/deactivate", 1)
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

	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("解压响应失败: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误 (状态码: %d)", resp.StatusCode)
	}

	var response DeactivateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
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
	deleteURL := strings.Replace(config.BaseURL, "/v1/hme/reserve", "/v1/hme/delete", 1)
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

	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("解压响应失败: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误 (状态码: %d)", resp.StatusCode)
	}

	var response PermanentDeleteResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
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
	reactivateURL := strings.Replace(config.BaseURL, "/v1/hme/reserve", "/v1/hme/reactivate", 1)
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

	for key, value := range config.Headers {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	var reader io.Reader = resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		gzipReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			return fmt.Errorf("解压响应失败: %v", err)
		}
		defer gzipReader.Close()
		reader = gzipReader
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("读取响应失败: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器返回错误 (状态码: %d)", resp.StatusCode)
	}

	var response ReactivateResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("解析响应失败: %v", err)
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
	var emails []string
	var errors []error
	var mu sync.Mutex

	printHeader("批量创建邮箱")
	fmt.Printf(ColorBold+ColorPurple+"将创建 %d 个邮箱，标签前缀: %s\n\n"+ColorReset, count, labelPrefix)

	for i := 0; i < count; i++ {
		label := fmt.Sprintf("%s%d", labelPrefix, i+1)

		// 显示进度条
		printProgressBar(i, count, "创建进度")

		fmt.Printf(ColorBlue+"正在创建邮箱 (标签: %s) ... "+ColorReset, label)

		email, err := createHME(config, label)

		mu.Lock()
		if err != nil {
			printError(fmt.Sprintf("创建失败: %v", err))
			errors = append(errors, err)
		} else {
			printSuccess(fmt.Sprintf("创建成功: %s", email))
			emails = append(emails, email)
		}
		mu.Unlock()

		// 延迟
		if i < count-1 && config.DelaySeconds > 0 {
			fmt.Printf(ColorYellow+"等待 %d 秒...\n"+ColorReset, config.DelaySeconds)
			time.Sleep(time.Duration(config.DelaySeconds) * time.Second)
		}
		fmt.Println()
	}

	// 完成进度条
	printProgressBar(count, count, "创建进度")
	fmt.Println()

	return emails, errors
}

// ANSI 颜色代码
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

// UI 辅助函数
func printSeparator() {
	fmt.Println(ColorBlue + strings.Repeat("=", 70) + ColorReset)
}

func printHeader(title string) {
	printSeparator()
	fmt.Printf(ColorBold+ColorCyan+"  %s\n"+ColorReset, title)
	printSeparator()
}

func printSubHeader(title string) {
	fmt.Println(ColorBlue + strings.Repeat("-", 50) + ColorReset)
	fmt.Printf(ColorBold+ColorBlue+"  %s\n"+ColorReset, title)
	fmt.Println(ColorBlue + strings.Repeat("-", 50) + ColorReset)
}

func printSuccess(message string) {
	fmt.Printf(ColorGreen+"[成功] %s\n"+ColorReset, message)
}

func printError(message string) {
	fmt.Printf(ColorRed+"[错误] %s\n"+ColorReset, message)
}

func printWarning(message string) {
	fmt.Printf(ColorYellow+"[警告] %s\n"+ColorReset, message)
}

func printInfo(message string) {
	fmt.Printf(ColorCyan+"[信息] %s\n"+ColorReset, message)
}

func printProgressBar(current, total int, prefix string) {
	barWidth := 30
	progress := float64(current) / float64(total)
	filled := int(progress * float64(barWidth))

	filledBar := ColorGreen + strings.Repeat("█", filled) + ColorReset
	emptyBar := ColorWhite + strings.Repeat("░", barWidth-filled) + ColorReset
	bar := filledBar + emptyBar
	percentage := int(progress * 100)

	fmt.Printf("\r"+ColorCyan+"%s"+ColorReset+" [%s] "+ColorBold+"%d%%"+ColorReset+" (%d/%d)", prefix, bar, percentage, current, total)
	if current == total {
		fmt.Println()
	}
}

func readInput(prompt string) string {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	return strings.TrimSpace(input)
}

func readInt(prompt string) (int, error) {
	input := readInput(prompt)
	return strconv.Atoi(input)
}

func confirmAction(message string) bool {
	fmt.Printf("%s (y/n): ", message)
	input := readInput("")
	return strings.ToLower(input) == "y" || strings.ToLower(input) == "yes"
}

// 保存邮箱到文件
func saveEmailsToFile(emails []string, filename string) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		printError(fmt.Sprintf("无法打开文件 %s: %v", filename, err))
		return
	}
	defer file.Close()

	for _, email := range emails {
		_, err := file.WriteString(email + "\n")
		if err != nil {
			printError(fmt.Sprintf("写入文件失败: %v", err))
			return
		}
	}

	printSuccess(fmt.Sprintf("邮箱地址已保存到: %s", filename))
}

// 显示主菜单
func showMainMenu() {
	printHeader("iCloud 隐藏邮箱管理工具")
	fmt.Println(ColorGreen + "  [1] 查看邮箱列表" + ColorReset)
	fmt.Println(ColorBlue + "  [2] 创建新邮箱" + ColorReset)
	fmt.Println(ColorYellow + "  [3] 停用邮箱" + ColorReset)
	fmt.Println(ColorPurple + "  [4] 批量创建邮箱" + ColorReset)
	fmt.Println(ColorRed + "  [5] 彻底删除停用的邮箱（不可恢复！）" + ColorReset)
	fmt.Println(ColorCyan + "  [6] 重新激活停用的邮箱" + ColorReset)
	fmt.Println(ColorWhite + "  [0] 退出" + ColorReset)
	printSeparator()
}

// 查看邮箱列表
func handleListEmails(config *Config) {
	printHeader("邮箱列表")
	fmt.Print(ColorCyan + "正在获取邮箱列表" + ColorReset)

	// 显示加载动画
	for i := 0; i < 3; i++ {
		fmt.Print(ColorBlue + "." + ColorReset)
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println()

	emails, err := listHME(config)
	if err != nil {
		printError(fmt.Sprintf("获取邮箱列表失败: %v", err))
		return
	}

	if len(emails) == 0 {
		printWarning("暂无邮箱")
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

	fmt.Printf("\n"+ColorBold+ColorPurple+"[统计] 总计 %d 个 | 激活 %d 个 | 停用 %d 个\n\n"+ColorReset,
		len(emails), activeCount, deactivatedCount)

	for i, email := range emails {
		statusColor := ColorGreen
		statusText := "激活"
		if !email.IsActive {
			statusColor = ColorYellow
			statusText = "已停用"
		}

		fmt.Printf("  "+ColorBold+"[%d]"+ColorReset+" %s%s"+ColorReset+" %s\n", i+1, statusColor, statusText, email.HME)
		fmt.Printf("      "+ColorCyan+"标签:"+ColorReset+" %s | "+ColorBlue+"状态:"+ColorReset+" %s%s"+ColorReset+"\n", email.Label, statusColor, statusText)
		if email.ForwardToEmail != "" {
			fmt.Printf("      "+ColorPurple+"转发至:"+ColorReset+" %s\n", email.ForwardToEmail)
		}

		// 显示创建时间
		createTime := time.Unix(email.CreateTimestamp/1000, 0)
		fmt.Printf("      "+ColorWhite+"创建时间:"+ColorReset+" %s\n", createTime.Format("2006-01-02 15:04:05"))
		fmt.Println()
	}
}

// 创建单个邮箱
func handleCreateEmail(config *Config) {
	printHeader("创建新邮箱")

	label := readInput(ColorCyan + "请输入邮箱标签: " + ColorReset)
	if label == "" {
		printError("标签不能为空")
		return
	}

	fmt.Printf("\n"+ColorBlue+"正在创建邮箱 (标签: %s)"+ColorReset, label)

	// 显示加载动画
	for i := 0; i < 3; i++ {
		fmt.Print(ColorBlue + "." + ColorReset)
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println()

	email, err := createHME(config, label)
	if err != nil {
		printError(fmt.Sprintf("邮箱创建失败: %v", err))
		return
	}

	printSuccess("邮箱创建成功!")
	fmt.Printf(ColorPurple+"邮箱地址:"+ColorReset+" %s\n", email)
	fmt.Printf(ColorCyan+"标签:"+ColorReset+" %s\n", label)
	fmt.Printf(ColorWhite+"创建时间:"+ColorReset+" %s\n", time.Now().Format("2006-01-02 15:04:05"))
}

// 停用邮箱
func handleDeleteEmails(config *Config) {
	printHeader("停用邮箱")
	fmt.Print(ColorCyan + "正在获取邮箱列表" + ColorReset)

	// 显示加载动画
	for i := 0; i < 3; i++ {
		fmt.Print(ColorBlue + "." + ColorReset)
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println()

	emails, err := listHME(config)
	if err != nil {
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
		printWarning("暂无激活的邮箱可停用")
		return
	}

	fmt.Printf("\n"+ColorBold+ColorPurple+"[统计] 共有 %d 个激活的邮箱:\n\n"+ColorReset, len(activeEmails))

	for i, email := range activeEmails {
		fmt.Printf("  "+ColorBold+"[%d]"+ColorReset+" "+ColorGreen+"激活"+ColorReset+" %s\n", i+1, email.HME)
		fmt.Printf("      "+ColorCyan+"标签:"+ColorReset+" %s\n", email.Label)
		fmt.Println()
	}

	printInfo("请输入要停用的邮箱序号 (多个序号用逗号分隔，如: 1,3,5)")
	input := readInput(ColorCyan + "序号: " + ColorReset)

	if input == "" {
		printWarning("操作已取消")
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
	fmt.Printf("\n"+ColorBold+ColorYellow+"[计划] 将要停用 %d 个邮箱:\n"+ColorReset, len(toDeactivate))
	for _, email := range toDeactivate {
		fmt.Printf("  "+ColorYellow+"●"+ColorReset+" %s (标签: %s)\n", email.HME, email.Label)
	}

	printWarning("停用后可以稍后重新激活")
	if !confirmAction("\n" + ColorBold + "确认停用这些邮箱吗?" + ColorReset) {
		printWarning("操作已取消")
		return
	}

	// 执行停用
	fmt.Println()
	printSubHeader("执行停用操作")
	successCount := 0
	failCount := 0

	for i, email := range toDeactivate {
		printProgressBar(i, len(toDeactivate), "停用进度")
		fmt.Printf(ColorBlue+"正在停用: %s ... "+ColorReset, email.HME)

		err := deactivateHME(config, email.AnonymousID)
		if err != nil {
			printError(fmt.Sprintf("停用失败: %v", err))
			failCount++
		} else {
			printSuccess("停用成功")
			successCount++
		}

		if i < len(toDeactivate)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 完成进度条
	printProgressBar(len(toDeactivate), len(toDeactivate), "停用进度")

	printSeparator()
	if successCount > 0 {
		printSuccess(fmt.Sprintf("停用完成: 成功 %d 个", successCount))
	}
	if failCount > 0 {
		printError(fmt.Sprintf("失败 %d 个", failCount))
	}
}

// 批量创建邮箱
func handleBatchCreate(config *Config) {
	printHeader("批量创建邮箱")

	count, err := readInt(ColorCyan + "请输入要创建的数量: " + ColorReset)
	if err != nil || count <= 0 {
		printError("无效的数量，请输入大于0的整数")
		return
	}

	if count > 50 {
		printWarning("建议单次创建数量不超过50个，避免被限制")
		if !confirmAction(ColorBold + "确认要创建这么多邮箱吗?" + ColorReset) {
			printWarning("操作已取消")
			return
		}
	}

	labelPrefix := readInput(ColorCyan + "请输入标签前缀 (默认: auto-): " + ColorReset)
	if labelPrefix == "" {
		labelPrefix = "auto-"
	}

	fmt.Printf("\n" + ColorBold + ColorPurple + "[计划] 批量创建计划:\n" + ColorReset)
	fmt.Printf("  "+ColorGreen+"●"+ColorReset+" 数量: %d 个邮箱\n", count)
	fmt.Printf("  "+ColorBlue+"●"+ColorReset+" 标签: %s1, %s2, %s3, ...\n", labelPrefix, labelPrefix, labelPrefix)
	fmt.Printf("  "+ColorYellow+"●"+ColorReset+" 延迟: %d 秒/个\n", config.DelaySeconds)

	estimatedTime := count * config.DelaySeconds
	fmt.Printf("  "+ColorCyan+"●"+ColorReset+" 预计耗时: %d 分 %d 秒\n", estimatedTime/60, estimatedTime%60)

	if !confirmAction("\n" + ColorBold + "确认开始批量创建吗?" + ColorReset) {
		printWarning("操作已取消")
		return
	}

	emails, errors := batchGenerate(config, count, labelPrefix)

	printSeparator()
	if len(emails) > 0 {
		printSuccess(fmt.Sprintf("批量创建完成: 成功 %d 个", len(emails)))
	}
	if len(errors) > 0 {
		printError(fmt.Sprintf("失败 %d 个", len(errors)))
	}

	if len(emails) > 0 {
		fmt.Println("\n" + ColorBold + ColorGreen + "[结果] 成功创建的邮箱:" + ColorReset)
		for i, email := range emails {
			fmt.Printf("  "+ColorGreen+"%d."+ColorReset+" %s\n", i+1, email)
		}

		// 保存到文件
		if config.OutputFile != "" {
			saveEmailsToFile(emails, config.OutputFile)
		}
	}
}

// 彻底删除停用的邮箱
func handlePermanentDelete(config *Config) {
	printHeader("彻底删除停用的邮箱（不可恢复！）")
	printWarning("此操作将永久删除邮箱，无法恢复！")

	fmt.Print(ColorCyan + "正在获取邮箱列表" + ColorReset)

	// 显示加载动画
	for i := 0; i < 3; i++ {
		fmt.Print(ColorBlue + "." + ColorReset)
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println()

	emails, err := listHME(config)
	if err != nil {
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

	fmt.Printf("\n"+ColorBold+ColorPurple+"[统计] 共有 %d 个已停用的邮箱:\n\n"+ColorReset, len(deactivatedEmails))

	for i, email := range deactivatedEmails {
		fmt.Printf("  "+ColorBold+"[%d]"+ColorReset+" "+ColorYellow+"已停用"+ColorReset+" %s\n", i+1, email.HME)
		fmt.Printf("      "+ColorCyan+"标签:"+ColorReset+" %s\n", email.Label)
		fmt.Println()
	}

	printInfo("请输入要彻底删除的邮箱序号 (多个序号用逗号分隔，如: 1,3,5)")
	input := readInput(ColorCyan + "序号: " + ColorReset)

	if input == "" {
		printWarning("操作已取消")
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
	fmt.Printf("\n"+ColorBold+ColorRed+"[危险] 将要彻底删除 %d 个邮箱:\n"+ColorReset, len(toDelete))
	for _, email := range toDelete {
		fmt.Printf("  "+ColorRed+"●"+ColorReset+" %s (标签: %s)\n", email.HME, email.Label)
	}

	printWarning("警告：此操作不可恢复！")
	fmt.Print("\n" + ColorBold + ColorRed + "确认彻底删除吗? 请输入 'DELETE' 确认: " + ColorReset)
	confirm := readInput("")

	if confirm != "DELETE" {
		printWarning("操作已取消")
		return
	}

	// 执行彻底删除
	fmt.Println()
	printSubHeader("执行彻底删除操作")
	successCount := 0
	failCount := 0

	for i, email := range toDelete {
		printProgressBar(i, len(toDelete), "删除进度")
		fmt.Printf(ColorRed+"正在删除: %s ... "+ColorReset, email.HME)

		err := permanentDeleteHME(config, email.AnonymousID)
		if err != nil {
			printError(fmt.Sprintf("删除失败: %v", err))
			failCount++
		} else {
			printSuccess("删除成功")
			successCount++
		}

		if i < len(toDelete)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 完成进度条
	printProgressBar(len(toDelete), len(toDelete), "删除进度")

	printSeparator()
	if successCount > 0 {
		printSuccess(fmt.Sprintf("彻底删除完成: 成功 %d 个", successCount))
	}
	if failCount > 0 {
		printError(fmt.Sprintf("失败 %d 个", failCount))
	}
}

// 重新激活停用的邮箱
func handleReactivate(config *Config) {
	printHeader("重新激活停用的邮箱")
	fmt.Print(ColorCyan + "正在获取邮箱列表" + ColorReset)

	// 显示加载动画
	for i := 0; i < 3; i++ {
		fmt.Print(ColorBlue + "." + ColorReset)
		time.Sleep(300 * time.Millisecond)
	}
	fmt.Println()

	emails, err := listHME(config)
	if err != nil {
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

	fmt.Printf("\n"+ColorBold+ColorPurple+"[统计] 共有 %d 个已停用的邮箱:\n\n"+ColorReset, len(deactivatedEmails))

	for i, email := range deactivatedEmails {
		fmt.Printf("  "+ColorBold+"[%d]"+ColorReset+" "+ColorYellow+"已停用"+ColorReset+" %s\n", i+1, email.HME)
		fmt.Printf("      "+ColorCyan+"标签:"+ColorReset+" %s\n", email.Label)
		fmt.Println()
	}

	printInfo("请输入要重新激活的邮箱序号 (多个序号用逗号分隔，如: 1,3,5)")
	input := readInput(ColorCyan + "序号: " + ColorReset)

	if input == "" {
		printWarning("操作已取消")
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
	fmt.Printf("\n"+ColorBold+ColorGreen+"[计划] 将要重新激活 %d 个邮箱:\n"+ColorReset, len(toReactivate))
	for _, email := range toReactivate {
		fmt.Printf("  "+ColorGreen+"●"+ColorReset+" %s (标签: %s)\n", email.HME, email.Label)
	}

	if !confirmAction("\n" + ColorBold + "确认重新激活这些邮箱吗?" + ColorReset) {
		printWarning("操作已取消")
		return
	}

	// 执行重新激活
	fmt.Println()
	printSubHeader("执行重新激活操作")
	successCount := 0
	failCount := 0

	for i, email := range toReactivate {
		printProgressBar(i, len(toReactivate), "激活进度")
		fmt.Printf(ColorGreen+"正在激活: %s ... "+ColorReset, email.HME)

		err := reactivateHME(config, email.AnonymousID)
		if err != nil {
			printError(fmt.Sprintf("激活失败: %v", err))
			failCount++
		} else {
			printSuccess("激活成功")
			successCount++
		}

		if i < len(toReactivate)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 完成进度条
	printProgressBar(len(toReactivate), len(toReactivate), "激活进度")

	printSeparator()
	if successCount > 0 {
		printSuccess(fmt.Sprintf("重新激活完成: 成功 %d 个", successCount))
	}
	if failCount > 0 {
		printError(fmt.Sprintf("失败 %d 个", failCount))
	}
}

func main() {
	// 显示启动信息
	printHeader("iCloud 隐藏邮箱管理工具")
	fmt.Println(ColorCyan + "版本: v2.0" + ColorReset)
	fmt.Println(ColorBlue + "作者: yuzeguitarist" + ColorReset)
	fmt.Println(ColorPurple + "功能: 创建、管理、删除 iCloud 隐藏邮箱" + ColorReset)
	printSeparator()

	// 加载配置
	fmt.Print(ColorCyan + "正在加载配置文件" + ColorReset)
	for i := 0; i < 3; i++ {
		fmt.Print(ColorBlue + "." + ColorReset)
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Println()

	config, err := loadConfig("config.json")
	if err != nil {
		printError(fmt.Sprintf("加载配置失败: %v", err))
		printInfo("请确保 config.json 文件存在且格式正确")
		os.Exit(1)
	}

	printSuccess("配置加载成功")
	time.Sleep(500 * time.Millisecond)

	// 主循环
	for {
		showMainMenu()
		choice := readInput(ColorCyan + "请选择操作 (0-6): " + ColorReset)

		switch choice {
		case "1":
			handleListEmails(config)
		case "2":
			handleCreateEmail(config)
		case "3":
			handleDeleteEmails(config)
		case "4":
			handleBatchCreate(config)
		case "5":
			handlePermanentDelete(config)
		case "6":
			handleReactivate(config)
		case "0":
			printHeader("感谢使用")
			fmt.Println(ColorGreen + "再见!" + ColorReset)
			return
		default:
			printError("无效的选择，请输入 0-6 之间的数字")
		}

		fmt.Print("\n" + ColorYellow + "按回车键继续..." + ColorReset)
		readInput("")

		// 清屏效果
		for i := 0; i < 3; i++ {
			fmt.Println()
		}
	}
}
