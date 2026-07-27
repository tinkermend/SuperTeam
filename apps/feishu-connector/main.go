// feishu-connector:飞书 IM 投影通道的独立进程。
// 职责:维护长连接、事件去重、卡片渲染与投递、把入站动作翻译成控制平面 API 调用。
// 铁律:不连数据库、不持业务状态、不自行判权;Console 永远是完整操作面,
// 本进程挂掉只影响飞书通道的时效,不阻断任何业务流。
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/superteam/feishu-connector/internal/cpclient"
	"github.com/superteam/feishu-connector/internal/gateway"
	"github.com/superteam/feishu-connector/internal/inbound"
	"github.com/superteam/feishu-connector/internal/outbound"
	"github.com/superteam/feishu-connector/internal/session"
)

// connectorVersion 写入心跳,便于 Console 观察部署偏斜。
const connectorVersion = "feishu-connector-p1"

func main() {
	controlPlaneURL := envOr("CONTROL_PLANE_URL", "http://127.0.0.1:8080")
	token := strings.TrimSpace(os.Getenv("FEISHU_CONNECTOR_TOKEN"))
	if token == "" {
		log.Fatal("FEISHU_CONNECTOR_TOKEN is required (issue via POST /api/v1/admin/service-tokens)")
	}
	webOrigin := envOr("CONTROL_PLANE_WEB_ORIGIN", "http://127.0.0.1:3000")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cp := cpclient.New(controlPlaneURL, token)
	configs := bootstrapWithRetry(ctx, cp)
	if len(configs) == 0 {
		log.Fatal("no active feishu app configs; seed one via POST /api/v1/admin/feishu/app-configs")
	}

	var (
		gateways []*gateway.Gateway
		pollers  []*outbound.Poller
		mu       sync.Mutex
	)

	for _, cfg := range configs {
		cfg := cfg
		sessions := session.NewStore(30 * time.Minute)
		router := inbound.NewRouter(cp, sessions, cfg.ConfigID, webOrigin)
		gw := gateway.New(cfg.ConfigID, cfg.AppID, cfg.AppSecret, router)
		router.SetMessenger(gw)

		poller := outbound.NewPoller(cp, gw, webOrigin)
		mu.Lock()
		gateways = append(gateways, gw)
		pollers = append(pollers, poller)
		mu.Unlock()

		go poller.Run(ctx)

		go func() {
			log.Printf("[connector] starting long connection for app %s", cfg.AppID)
			if err := gw.Start(ctx, 4); err != nil && ctx.Err() == nil {
				log.Printf("[connector] long connection for app %s exited: %v", cfg.AppID, err)
			}
		}()
	}

	go runHeartbeat(ctx, cp, &mu, &gateways, &pollers)

	<-ctx.Done()
	log.Println("[connector] shutting down")
}

func runHeartbeat(ctx context.Context, cp *cpclient.Client, mu *sync.Mutex, gateways *[]*gateway.Gateway, pollers *[]*outbound.Poller) {
	// 立即报一次,再按 30s 周期。
	send := func() {
		mu.Lock()
		gws := append([]*gateway.Gateway(nil), (*gateways)...)
		pls := append([]*outbound.Poller(nil), (*pollers)...)
		mu.Unlock()

		apps := make([]cpclient.HeartbeatApp, 0, len(gws))
		for _, gw := range gws {
			appID, configID, wsStatus, lastEvent := gw.StatusSnapshot()
			item := cpclient.HeartbeatApp{
				AppID:    appID,
				ConfigID: configID,
				WSStatus: wsStatus,
			}
			if !lastEvent.IsZero() {
				t := lastEvent
				item.LastWSEventAt = &t
			}
			apps = append(apps, item)
		}
		var lastPoll *time.Time
		for _, p := range pls {
			if t := p.LastPollAt(); t != nil {
				if lastPoll == nil || t.After(*lastPoll) {
					lastPoll = t
				}
			}
		}
		if err := cp.Heartbeat(ctx, cpclient.HeartbeatRequest{
			Version:          connectorVersion,
			LastOutboxPollAt: lastPoll,
			Apps:             apps,
		}); err != nil && ctx.Err() == nil {
			log.Printf("[connector] heartbeat: %v", err)
		}
	}
	send()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			send()
		}
	}
}

// bootstrapWithRetry 启动期拉取应用配置,控制平面暂不可达时退避重试。
func bootstrapWithRetry(ctx context.Context, cp *cpclient.Client) []cpclient.BootstrapConfig {
	backoff := time.Second
	for {
		configs, err := cp.Bootstrap(ctx)
		if err == nil {
			return configs
		}
		log.Printf("[connector] bootstrap failed (retry in %s): %v", backoff, err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
