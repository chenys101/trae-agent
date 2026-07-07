package llm

import (
	"net"
	"net/http"

	"github.com/bytedance/trae-agent/internal/consts"
)

// httpClient 为 llm 包共享的 HTTP 客户端。
// 流式场景需要长连接，故不设置 Client.Timeout；转而在 Transport 层
// 设置连接/握手/响应头超时，避免请求卡死在建立连接阶段。
var httpClient = &http.Client{
	Transport: &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: consts.HTTPDialTimeout,
		}).DialContext,
		TLSHandshakeTimeout:   consts.HTTPTLSHandshakeTimeout,
		ResponseHeaderTimeout: consts.HTTPResponseHeaderTimeout,
	},
}
