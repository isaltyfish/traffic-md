package iputil

import (
	"net"
	"strings"
)

// IPType represents the type of IP address
type IPType int

const (
	IPTypePublic   IPType = iota // 公网IP
	IPTypePrivate                // 内网IP
	IPTypeLoopback               // 回环地址
	IPTypeOther                  // 其他（无法区分）
)

// ClassifyIP classifies an IP address as public, private, loopback, or other
func ClassifyIP(ipStr string) IPType {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return IPTypeOther
	}

	// Check for loopback addresses
	if ip.IsLoopback() {
		return IPTypeLoopback
	}

	// Check for private IP ranges
	if ip.IsPrivate() {
		return IPTypePrivate
	}

	// Check for link-local addresses
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return IPTypePrivate
	}

	// Check for multicast addresses
	if ip.IsMulticast() {
		return IPTypeOther
	}

	// Check for unspecified address
	if ip.IsUnspecified() {
		return IPTypeOther
	}

	// IPv4 specific checks
	if ip4 := ip.To4(); ip4 != nil {
		// Check for private ranges manually (for older Go versions)
		ip4Str := ip4.String()
		// RFC 1918 private addresses
		if strings.HasPrefix(ip4Str, "10.") ||
			strings.HasPrefix(ip4Str, "172.16.") ||
			strings.HasPrefix(ip4Str, "172.17.") ||
			strings.HasPrefix(ip4Str, "172.18.") ||
			strings.HasPrefix(ip4Str, "172.19.") ||
			strings.HasPrefix(ip4Str, "172.20.") ||
			strings.HasPrefix(ip4Str, "172.21.") ||
			strings.HasPrefix(ip4Str, "172.22.") ||
			strings.HasPrefix(ip4Str, "172.23.") ||
			strings.HasPrefix(ip4Str, "172.24.") ||
			strings.HasPrefix(ip4Str, "172.25.") ||
			strings.HasPrefix(ip4Str, "172.26.") ||
			strings.HasPrefix(ip4Str, "172.27.") ||
			strings.HasPrefix(ip4Str, "172.28.") ||
			strings.HasPrefix(ip4Str, "172.29.") ||
			strings.HasPrefix(ip4Str, "172.30.") ||
			strings.HasPrefix(ip4Str, "172.31.") ||
			strings.HasPrefix(ip4Str, "192.168.") ||
			strings.HasPrefix(ip4Str, "169.254.") {
			return IPTypePrivate
		}
		// RFC 6598 Carrier-Grade NAT (100.64.0.0/10)
		// 100.64.0.0 to 100.127.255.255
		// 注意：虽然技术上这些地址不可路由，但从服务器角度看，它们是"外部"流量
		// 因此归类为公网地址，而不是私有地址
		// 如果 IP 在 100.64.0.0/10 范围内，继续处理，最终会归类为公网
		_ = ip4 // 保留此检查点，但不再返回 Private
	}

	// If it's a valid IP and not private/loopback, consider it public
	return IPTypePublic
}

// IsPublicIP checks if an IP is public
func IsPublicIP(ipStr string) bool {
	return ClassifyIP(ipStr) == IPTypePublic
}

// IsPrivateIP checks if an IP is private (including loopback)
func IsPrivateIP(ipStr string) bool {
	t := ClassifyIP(ipStr)
	return t == IPTypePrivate || t == IPTypeLoopback
}
