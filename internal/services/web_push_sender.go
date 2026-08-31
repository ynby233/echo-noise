package services

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/rcy1314/echo-noise/internal/models"
)

var defaultWebPushHTTPClient = newWebPushHTTPClient()

func isPublicWebPushIP(ip net.IP) bool {
	return ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() &&
		!ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsMulticast()
}

func webPushDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errors.New("Web Push 目标地址无效")
	}
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return nil, errors.New("Web Push 目标地址无效")
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("解析 Web Push 目标失败: %w", err)
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	for _, candidate := range addresses {
		if !isPublicWebPushIP(candidate.IP) {
			continue
		}
		connection, dialErr := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		err = dialErr
	}
	if err != nil {
		return nil, fmt.Errorf("连接 Web Push 目标失败: %w", err)
	}
	return nil, errors.New("Web Push 目标不是公网地址")
}

func newWebPushHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = webPushDialContext
	return &http.Client{
		Transport: transport,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

type VAPIDWebPushSender struct {
	Config WebPushRuntimeConfig
	Client *http.Client
}

func (sender VAPIDWebPushSender) Send(ctx context.Context, subscription models.WebPushSubscription, payload []byte) (WebPushSendResult, error) {
	if !sender.Config.Ready() {
		return WebPushSendResult{}, errors.New("Web Push 尚未配置")
	}
	client := sender.Client
	if client == nil {
		client = defaultWebPushHTTPClient
	}
	response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: subscription.P256dh,
			Auth:   subscription.Auth,
		},
	}, &webpush.Options{
		Subscriber:      sender.Config.Subject,
		VAPIDPublicKey:  sender.Config.PublicKey,
		VAPIDPrivateKey: sender.Config.PrivateKey,
		TTL:             24 * 60 * 60,
		HTTPClient:      client,
	})
	if err != nil {
		return WebPushSendResult{}, err
	}
	defer response.Body.Close()
	return WebPushSendResult{StatusCode: response.StatusCode}, nil
}
