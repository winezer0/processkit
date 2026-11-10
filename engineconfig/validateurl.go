package engineconfig

import (
	"fmt"
	"net/url"
	"strings"
)

const maximumURLBytes = 2048

// validateURL 校验代理URL，仅允许绝对 http, https, socks5 地址
// 限制：长度不超过 maximumURLBytes，不包含空白字符(\r \n \t 空格)
func validateURL(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > maximumURLBytes {
		return fmt.Errorf("url length must not exceed %d bytes", maximumURLBytes)
	}
	// 禁止内嵌空白字符
	if strings.ContainsAny(value, "\r\n\t ") {
		return fmt.Errorf("url contains invalid whitespace characters")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil {
		return fmt.Errorf("invalid url format: %w", err)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("url missing host")
	}
	switch parsed.Scheme {
	case "http", "https", "socks5":
	default:
		return fmt.Errorf("unsupported scheme %q, only http, https, socks5 are allowed", parsed.Scheme)
	}
	return nil
}
