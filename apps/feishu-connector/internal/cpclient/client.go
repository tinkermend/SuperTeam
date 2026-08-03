// Package cpclient 是控制平面 API 客户端:服务凭据认证,业务动作带
// on-behalf-of 头(行为人=绑定用户)。connector 全部业务事实经此进出,
// 本进程不持任何业务状态。
package cpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL     string
	token       string
	serviceName string
	httpClient  *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		token:       token,
		serviceName: "feishu-connector",
		// 覆盖 outbox 长轮询 wait_ms(默认 2s)+网络余量;普通请求仍远短于该上限。
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// OnBehalfOf 标识一次请求的行为人(绑定用户+其 open_id,服务端反查核验)。
type OnBehalfOf struct {
	UserID string
	OpenID string
}

type APIError struct {
	Status int
	Body   string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("control plane api error: status=%d body=%s", e.Status, e.Body)
}

func (c *Client) do(ctx context.Context, method, path string, obo *OnBehalfOf, payload, out any) error {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("X-Service-Name", c.serviceName)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if obo != nil {
		req.Header.Set("X-On-Behalf-Of", obo.UserID)
		req.Header.Set("X-Feishu-Open-Id", obo.OpenID)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		// 64KB:409 冲突响应携带决策卡快照(card_payload),2KB 会截断富上下文。
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return &APIError{Status: resp.StatusCode, Body: string(raw)}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type BootstrapConfig struct {
	ConfigID  string `json:"config_id"`
	TenantID  string `json:"tenant_id"`
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

func (c *Client) Bootstrap(ctx context.Context) ([]BootstrapConfig, error) {
	var out struct {
		Configs []BootstrapConfig `json:"configs"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/connector/bootstrap", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Configs, nil
}

type Identity struct {
	AuthUserID string `json:"auth_user_id"`
	OpenID     string `json:"open_id"`
	BoundVia   string `json:"bound_via"`
}

// ResolveIdentity 按 open_id 反查绑定;未绑定返回 (nil, nil)。
func (c *Client) ResolveIdentity(ctx context.Context, appConfigID, openID string) (*Identity, error) {
	query := url.Values{"app_config_id": {appConfigID}, "open_id": {openID}}
	var out Identity
	err := c.do(ctx, http.MethodGet, "/api/v1/connector/identity?"+query.Encode(), nil, nil, &out)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &out, nil
}

type OutboxItem struct {
	ID              string         `json:"id"`
	Kind            string         `json:"kind"`
	ResourceType    string         `json:"resource_type"`
	ResourceID      string         `json:"resource_id"`
	ProjectID       string         `json:"project_id,omitempty"`
	RecipientUserID string         `json:"recipient_user_id"`
	RecipientOpenID string         `json:"recipient_open_id"`
	Payload         map[string]any `json:"payload"`
	Attempts        int32          `json:"attempts"`
}

// ListOutbox 拉取 pending outbox。waitMs>0 时控制平面长轮询:空队列挂起到有写入或超时。
func (c *Client) ListOutbox(ctx context.Context, limit int) ([]OutboxItem, error) {
	return c.ListOutboxWait(ctx, limit, 0)
}

// ListOutboxWait 同 ListOutbox,waitMs 传给控制平面 wait_ms(0 表示立即返回)。
func (c *Client) ListOutboxWait(ctx context.Context, limit, waitMs int) ([]OutboxItem, error) {
	var out struct {
		Items []OutboxItem `json:"items"`
	}
	path := fmt.Sprintf("/api/v1/connector/outbox?limit=%d", limit)
	if waitMs > 0 {
		path = fmt.Sprintf("%s&wait_ms=%d", path, waitMs)
	}
	// 长轮询时 HTTP client 超时需覆盖 wait_ms;do 使用 ctx,由调用方控制。
	if err := c.do(ctx, http.MethodGet, path, nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

func (c *Client) AckOutbox(ctx context.Context, id, result, feishuMessageID, errText string) error {
	payload := map[string]string{"result": result}
	if feishuMessageID != "" {
		payload["feishu_message_id"] = feishuMessageID
	}
	if errText != "" {
		payload["error"] = errText
	}
	return c.do(ctx, http.MethodPost, "/api/v1/connector/outbox/"+id+"/ack", nil, payload, nil)
}

type HeartbeatApp struct {
	AppID         string     `json:"app_id"`
	ConfigID      string     `json:"config_id,omitempty"`
	WSStatus      string     `json:"ws_status"`
	LastWSEventAt *time.Time `json:"last_ws_event_at,omitempty"`
}

type HeartbeatRequest struct {
	Version          string         `json:"version"`
	LastOutboxPollAt *time.Time     `json:"last_outbox_poll_at,omitempty"`
	Apps             []HeartbeatApp `json:"apps"`
}

func (c *Client) Heartbeat(ctx context.Context, req HeartbeatRequest) error {
	return c.do(ctx, http.MethodPost, "/api/v1/connector/heartbeat", nil, req, nil)
}

type MyProject struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *Client) MyProjects(ctx context.Context, obo OnBehalfOf) ([]MyProject, error) {
	var out struct {
		Projects []MyProject `json:"projects"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/connector/my-projects", &obo, nil, &out); err != nil {
		return nil, err
	}
	return out.Projects, nil
}

type SubmitDemandRequest struct {
	ProjectID        string `json:"project_id"`
	Title            string `json:"title"`
	Content          string `json:"content"`
	CoordinationMode string `json:"coordination_mode"`
}

type SubmitDemandResponse struct {
	DemandID string `json:"demand_id"`
	Title    string `json:"title"`
	Status   string `json:"status"`
}

func (c *Client) SubmitDemand(ctx context.Context, obo OnBehalfOf, req SubmitDemandRequest) (*SubmitDemandResponse, error) {
	var out SubmitDemandResponse
	if err := c.do(ctx, http.MethodPost, "/api/v1/connector/demands", &obo, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type ResolveDecisionRequest struct {
	ProjectID string `json:"project_id"`
	Decision  string `json:"decision"`
	Comment   string `json:"comment,omitempty"`
}

type resolveDecisionResponse struct {
	Status      string         `json:"status"`
	CardPayload map[string]any `json:"card_payload"`
}

type SignCriterionRequest struct {
	ProjectID   string `json:"project_id"`
	DecisionID  string `json:"decision_id,omitempty"`
	CriterionID string `json:"criterion_id"`
	Verdict     string `json:"verdict"`
	Reason      string `json:"reason,omitempty"`
}

// SignCriterionOutcome 是卡内签署响应:进度 + 判据 verdict 覆盖 + 刷新卡快照。
type SignCriterionOutcome struct {
	DemandStatus      string            `json:"demand_status"`
	Signed            int32             `json:"signed"`
	Total             int32             `json:"total"`
	Remaining         int32             `json:"remaining"`
	CriterionVerdicts map[string]string `json:"criterion_verdicts"`
	CardPayload       map[string]any    `json:"card_payload"`
}

// SignCriterion 卡内逐条签署验收判据(on-behalf-of)。409 = 需求不在待验收态。
func (c *Client) SignCriterion(ctx context.Context, obo OnBehalfOf, demandID string, req SignCriterionRequest) (*SignCriterionOutcome, error) {
	var out SignCriterionOutcome
	if err := c.do(ctx, http.MethodPost, "/api/v1/connector/demands/"+demandID+"/criteria/sign", &obo, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResolveDecision 回传决策;409 表示已由他人处理(调用方渲染对应卡态)。
// cardPayload 是控制平面返回的决策卡快照(与 outbox 决策卡同源),供即时置换
// 渲染保留详情的终态卡;两条路径都 best-effort,可能为 nil。
func (c *Client) ResolveDecision(ctx context.Context, obo OnBehalfOf, decisionID string, req ResolveDecisionRequest) (cardPayload map[string]any, conflict bool, err error) {
	var out resolveDecisionResponse
	err = c.do(ctx, http.MethodPost, "/api/v1/connector/decisions/"+decisionID+"/resolve", &obo, req, &out)
	if err != nil {
		var apiErr *APIError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
			var body resolveDecisionResponse
			_ = json.Unmarshal([]byte(apiErr.Body), &body)
			return body.CardPayload, true, nil
		}
		return nil, false, err
	}
	return out.CardPayload, false, nil
}
