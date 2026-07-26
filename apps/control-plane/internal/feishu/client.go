package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client 是飞书开放平台 HTTP 客户端的最小实现:tenant_access_token、
// 通讯录邮箱反查 open_id、OAuth code 换用户身份。baseURL 可注入(测试用 httptest)。
type Client struct {
	baseURL    string
	httpClient *http.Client
}

var ErrFeishuAPI = errors.New("feishu api error")

const DefaultBaseURL = "https://open.feishu.cn"

func NewClient(baseURL string) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

type tenantTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
}

// TenantAccessToken 获取应用级租户凭证(调用方自行做短期缓存;P1 每次现取)。
func (c *Client) TenantAccessToken(ctx context.Context, appID, appSecret string) (string, error) {
	payload := map[string]string{"app_id": appID, "app_secret": appSecret}
	var out tenantTokenResponse
	if err := c.postJSON(ctx, "/open-apis/auth/v3/tenant_access_token/internal", "", payload, &out); err != nil {
		return "", err
	}
	if out.Code != 0 || out.TenantAccessToken == "" {
		return "", fmt.Errorf("%w: tenant_access_token code=%d msg=%s", ErrFeishuAPI, out.Code, out.Msg)
	}
	return out.TenantAccessToken, nil
}

type batchGetIDResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		UserList []struct {
			UserID string `json:"user_id"`
			Email  string `json:"email"`
			Mobile string `json:"mobile"`
		} `json:"user_list"`
	} `json:"data"`
}

// BatchGetOpenIDs 按邮箱与手机号批量反查 open_id,一次请求两组键(batch_get_id
// 原生同时接受 emails/mobiles 数组;单组每次上限 50,现状用户量内不分片,对齐
// 既有实现)。返回两个命中映射:email→open_id 与 mobile→open_id。手机号键以
// 请求发出的原值回带比对(飞书按档案手机号命中,格式需与档案一致,通常含区号)。
func (c *Client) BatchGetOpenIDs(ctx context.Context, tenantToken string, emails, mobiles []string) (map[string]string, map[string]string, error) {
	if len(emails) == 0 && len(mobiles) == 0 {
		return map[string]string{}, map[string]string{}, nil
	}
	payload := map[string]any{}
	if len(emails) > 0 {
		payload["emails"] = emails
	}
	if len(mobiles) > 0 {
		payload["mobiles"] = mobiles
	}
	var out batchGetIDResponse
	if err := c.postJSON(ctx, "/open-apis/contact/v3/users/batch_get_id?user_id_type=open_id", tenantToken, payload, &out); err != nil {
		return nil, nil, err
	}
	if out.Code != 0 {
		return nil, nil, fmt.Errorf("%w: batch_get_id code=%d msg=%s", ErrFeishuAPI, out.Code, out.Msg)
	}
	emailMatches := make(map[string]string)
	mobileMatches := make(map[string]string)
	for _, entry := range out.Data.UserList {
		if entry.UserID == "" {
			continue
		}
		if entry.Email != "" {
			emailMatches[entry.Email] = entry.UserID
		}
		if entry.Mobile != "" {
			mobileMatches[entry.Mobile] = entry.UserID
		}
	}
	return emailMatches, mobileMatches, nil
}

// AuthorizeURL 拼装 OAuth 授权页地址(浏览器重定向目标)。
func (c *Client) AuthorizeURL(appID, redirectURI, state string) string {
	query := url.Values{}
	query.Set("app_id", appID)
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	return c.baseURL + "/open-apis/authen/v1/authorize?" + query.Encode()
}

type oauthTokenResponse struct {
	Code        int    `json:"code"`
	Msg         string `json:"msg"`
	AccessToken string `json:"access_token"`
}

type userInfoResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		OpenID  string `json:"open_id"`
		UnionID string `json:"union_id"`
	} `json:"data"`
}

// OAuthUserIdentity 用授权码换取用户身份(open_id/union_id)。
func (c *Client) OAuthUserIdentity(ctx context.Context, appID, appSecret, code, redirectURI string) (openID, unionID string, err error) {
	tokenPayload := map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     appID,
		"client_secret": appSecret,
		"code":          code,
		"redirect_uri":  redirectURI,
	}
	var tokenOut oauthTokenResponse
	if err := c.postJSON(ctx, "/open-apis/authen/v2/oauth/token", "", tokenPayload, &tokenOut); err != nil {
		return "", "", err
	}
	if tokenOut.AccessToken == "" {
		return "", "", fmt.Errorf("%w: oauth token code=%d msg=%s", ErrFeishuAPI, tokenOut.Code, tokenOut.Msg)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/open-apis/authen/v1/user_info", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+tokenOut.AccessToken)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var infoOut userInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&infoOut); err != nil {
		return "", "", err
	}
	if infoOut.Code != 0 || infoOut.Data.OpenID == "" {
		return "", "", fmt.Errorf("%w: user_info code=%d msg=%s", ErrFeishuAPI, infoOut.Code, infoOut.Msg)
	}
	return infoOut.Data.OpenID, infoOut.Data.UnionID, nil
}

// ProbeContactDirectory 探测通讯录读权限:用一个必不存在的手机号调 batch_get_id。
// code=0 表示权限与授权范围 API 可用(人是否命中另论);权限类错误码视为 scope 缺失。
func (c *Client) ProbeContactDirectory(ctx context.Context, tenantToken string) (ok bool, code int, msg string, err error) {
	var out batchGetIDResponse
	if err := c.postJSON(ctx, "/open-apis/contact/v3/users/batch_get_id?user_id_type=open_id", tenantToken, map[string]any{
		"mobiles": []string{"+8600000000000"},
	}, &out); err != nil {
		return false, 0, "", err
	}
	return out.Code == 0, out.Code, out.Msg, nil
}

type botInfoResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Bot  struct {
		ActivateStatus int    `json:"activate_status"`
		AppName        string `json:"app_name"`
	} `json:"bot"`
}

// ProbeBotInfo 探测 bot/发消息相关能力是否对应用开放(im 权限与 bot 启用态)。
func (c *Client) ProbeBotInfo(ctx context.Context, tenantToken string) (ok bool, code int, msg string, err error) {
	var out botInfoResponse
	if err := c.getJSON(ctx, "/open-apis/bot/v3/info", tenantToken, &out); err != nil {
		return false, 0, "", err
	}
	// bot 接口成功且应用已启用即可;具体"可用范围是否覆盖某人"只能在真实发卡时暴露。
	return out.Code == 0, out.Code, out.Msg, nil
}

func (c *Client) getJSON(ctx context.Context, path, bearer string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: status %d", ErrFeishuAPI, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) postJSON(ctx context.Context, path, bearer string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: status %d", ErrFeishuAPI, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
