package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestReverseProxyForwardsBodyWithoutContentLength 复现 WebView 资产服务器传入
// POST 请求的场景：Wails 用 http.NewRequestWithContext 基于平台流（Windows 的
// IStream / Linux 的 GInputStream）构造请求，body 是泛型 io.ReadCloser，
// ContentLength 保持 0 且请求头不含 Content-Length。修复前 httputil.ReverseProxy
// 将 ContentLength==0 视为无请求体并置 outreq.Body = nil，后端收到空 POST，
// BindJSON 失败并返回"请求参数无效"（创建书籍等所有带 body 的请求均受影响）。
func TestReverseProxyForwardsBodyWithoutContentLength(t *testing.T) {
	const payload = `{"title":"测试","author":"","description":"测试"}`
	var received []byte
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	proxy := newReverseProxy(backend.URL)

	// 与 Wails internal/assetserver.processWebViewRequestInternal 相同的构造方式：
	// io.NopCloser 隐藏具体 reader 类型，NewRequestWithContext 无法推断长度，
	// ContentLength 保持 0，请求头不含 Content-Length。
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/books/create", io.NopCloser(strings.NewReader(payload)))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	if req.ContentLength != 0 {
		t.Fatalf("前置条件不成立: ContentLength = %d, 期望 0", req.ContentLength)
	}

	rec := httptest.NewRecorder()
	proxy.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("代理响应码 = %d, 期望 200", rec.Code)
	}
	if string(received) != payload {
		t.Fatalf("后端收到请求体 = %q, 期望 %q", received, payload)
	}
}
