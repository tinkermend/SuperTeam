package authz

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type OpenFGAClientConfig struct {
	APIURL     string
	StoreID    string
	ModelID    string
	APIToken   string
	HTTPClient *http.Client
}

type OpenFGATupleWriter interface {
	WriteTuples(ctx context.Context, writes, deletes []OpenFGATuple) error
}

type OpenFGAHTTPClient struct {
	apiURL     string
	storeID    string
	modelID    string
	apiToken   string
	httpClient *http.Client
}

func NewOpenFGAHTTPClient(cfg OpenFGAClientConfig) *OpenFGAHTTPClient {
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &OpenFGAHTTPClient{
		apiURL:     strings.TrimRight(cfg.APIURL, "/"),
		storeID:    cfg.StoreID,
		modelID:    cfg.ModelID,
		apiToken:   cfg.APIToken,
		httpClient: client,
	}
}

func (c *OpenFGAHTTPClient) Check(ctx context.Context, check OpenFGACheck) (bool, error) {
	var response struct {
		Allowed bool `json:"allowed"`
	}
	err := c.postJSON(ctx, "/stores/"+c.storeID+"/check", map[string]any{
		"authorization_model_id": c.modelID,
		"tuple_key":              tupleKeyPayload(check.User, check.Relation, check.Object),
	}, http.StatusOK, &response)
	return response.Allowed, err
}

func (c *OpenFGAHTTPClient) WriteTuples(ctx context.Context, writes, deletes []OpenFGATuple) error {
	payload := map[string]any{"authorization_model_id": c.modelID}
	if len(writes) > 0 {
		payload["writes"] = map[string]any{
			"tuple_keys":   tuplePayloads(writes),
			"on_duplicate": "ignore",
		}
	}
	if len(deletes) > 0 {
		payload["deletes"] = map[string]any{
			"tuple_keys": tuplePayloads(deletes),
			"on_missing": "ignore",
		}
	}
	return c.postJSONAccepting(ctx, "/stores/"+c.storeID+"/write", payload, []int{http.StatusOK, http.StatusNoContent}, nil)
}

func (c *OpenFGAHTTPClient) postJSON(ctx context.Context, path string, payload any, expectedStatus int, out any) error {
	return c.postJSONAccepting(ctx, path, payload, []int{expectedStatus}, out)
}

func (c *OpenFGAHTTPClient) postJSONAccepting(ctx context.Context, path string, payload any, expectedStatuses []int, out any) error {
	if c == nil || c.apiURL == "" || c.storeID == "" {
		return fmt.Errorf("openfga client is not configured")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiToken)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if !statusAccepted(resp.StatusCode, expectedStatuses) {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("openfga %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func statusAccepted(status int, expectedStatuses []int) bool {
	for _, expected := range expectedStatuses {
		if status == expected {
			return true
		}
	}
	return false
}

func tuplePayloads(tuples []OpenFGATuple) []map[string]string {
	out := make([]map[string]string, 0, len(tuples))
	for _, tuple := range tuples {
		out = append(out, tupleKeyPayload(tuple.User, tuple.Relation, tuple.Object))
	}
	return out
}

func tupleKeyPayload(user, relation, object string) map[string]string {
	return map[string]string{
		"user":     user,
		"relation": relation,
		"object":   object,
	}
}
