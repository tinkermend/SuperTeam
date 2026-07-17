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
		} `json:"user_list"`
	} `json:"data"`
}

// BatchGetOpenIDsByEmail 按邮箱批量反查 open_id;只返回命中的映射(email→open_id)。
func (c *Client) BatchGetOpenIDsByEmail(ctx context.Context, tenantToken string, emails []string) (map[string]string, error) {
	if len(emails) == 0 {
		return map[string]string{}, nil
	}
	payload := map[string]any{"emails": emails}
	var out batchGetIDResponse
	if err := c.postJSON(ctx, "/open-apis/contact/v3/users/batch_get_id?user_id_type=open_id", tenantToken, payload, &out); err != nil {
		return nil, err
	}
	if out.Code != 0 {
		return nil, fmt.Errorf("%w: batch_get_id code=%d msg=%s", ErrFeishuAPI, out.Code, out.Msg)
	}
	matches := make(map[string]string)
	for _, entry := range out.Data.UserList {
		if entry.UserID != "" && entry.Email != "" {
			matches[entry.Email] = entry.UserID
		}
	}
	return matches, nil
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
