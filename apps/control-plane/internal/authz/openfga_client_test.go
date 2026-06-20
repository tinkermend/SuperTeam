package authz

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenFGAHTTPClientCheckSendsStoreModelAndToken(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_, _ = w.Write([]byte(`{"allowed":true}`))
	}))
	defer server.Close()
	client := NewOpenFGAHTTPClient(OpenFGAClientConfig{
		APIURL:   server.URL,
		StoreID:  "store-1",
		ModelID:  "model-1",
		APIToken: "token-1",
	})

	allowed, err := client.Check(context.Background(), OpenFGACheck{
		User:     "user:alice",
		Relation: "admin",
		Object:   "tenant:t1",
	})

	require.NoError(t, err)
	require.True(t, allowed)
	require.Equal(t, "/stores/store-1/check", gotPath)
	require.Equal(t, "Bearer token-1", gotAuth)
	require.Equal(t, "model-1", gotBody["authorization_model_id"])
	tupleKey := gotBody["tuple_key"].(map[string]any)
	require.Equal(t, "user:alice", tupleKey["user"])
	require.Equal(t, "admin", tupleKey["relation"])
	require.Equal(t, "tenant:t1", tupleKey["object"])
}

func TestOpenFGAHTTPClientWriteTuplesSendsWritesAndDeletes(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := NewOpenFGAHTTPClient(OpenFGAClientConfig{
		APIURL:  server.URL,
		StoreID: "store-1",
		ModelID: "model-1",
	})

	err := client.WriteTuples(context.Background(),
		[]OpenFGATuple{{User: "user:alice", Relation: "admin", Object: "tenant:t1"}},
		[]OpenFGATuple{{User: "user:bob", Relation: "viewer", Object: "tenant:t1"}},
	)

	require.NoError(t, err)
	require.Equal(t, "/stores/store-1/write", gotPath)
	require.Equal(t, "model-1", gotBody["authorization_model_id"])
	writes := gotBody["writes"].(map[string]any)["tuple_keys"].([]any)
	deletes := gotBody["deletes"].(map[string]any)["tuple_keys"].([]any)
	require.Len(t, writes, 1)
	require.Len(t, deletes, 1)
	require.Equal(t, "user:alice", writes[0].(map[string]any)["user"])
	require.Equal(t, "user:bob", deletes[0].(map[string]any)["user"])
}

func TestOpenFGAHTTPClientWriteTuplesAcceptsOKResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/stores/store-1/write", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := NewOpenFGAHTTPClient(OpenFGAClientConfig{
		APIURL:  server.URL,
		StoreID: "store-1",
		ModelID: "model-1",
	})

	err := client.WriteTuples(context.Background(),
		[]OpenFGATuple{{User: "user:alice", Relation: "admin", Object: "tenant:t1"}},
		nil,
	)

	require.NoError(t, err)
}

func TestOpenFGAHTTPClientWriteTuplesIgnoresDuplicateWritesAndMissingDeletes(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client := NewOpenFGAHTTPClient(OpenFGAClientConfig{
		APIURL:  server.URL,
		StoreID: "store-1",
		ModelID: "model-1",
	})

	err := client.WriteTuples(context.Background(),
		[]OpenFGATuple{{User: "user:alice", Relation: "admin", Object: "tenant:t1"}},
		[]OpenFGATuple{{User: "user:bob", Relation: "viewer", Object: "tenant:t1"}},
	)

	require.NoError(t, err)
	require.Equal(t, "ignore", gotBody["writes"].(map[string]any)["on_duplicate"])
	require.Equal(t, "ignore", gotBody["deletes"].(map[string]any)["on_missing"])
}
