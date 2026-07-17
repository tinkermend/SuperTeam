package projectcoordination

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSecretLeakDetectorFires(t *testing.T) {
	d := newSecretLeakDetector()

	cases := []struct {
		name string
		art  DetectionArtifact
	}{
		{
			name: "openai-style key in diff",
			art: DetectionArtifact{
				DiffText: "+const apiKey = \"sk-abcdef0123456789ABCD\"\n",
			},
		},
		{
			name: "aws key in diff",
			art: DetectionArtifact{
				DiffText: "+aws_access_key_id = AKIAABCDEFGHIJKLMNOP\n",
			},
		},
		{
			name: "pem private key header in diff",
			art: DetectionArtifact{
				DiffText: "+-----BEGIN RSA PRIVATE KEY-----\n+MIIEpAIBAAKCAQEA...\n",
			},
		},
		{
			name: "inline password assignment",
			art: DetectionArtifact{
				DiffText: "+password = \"hunter2superSecret\"\n",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := d.Detect(context.Background(), tc.art)
			if !res.Detected {
				t.Fatalf("expected Detected=true, got false")
			}
			if res.Severity != "block" {
				t.Fatalf("expected Severity=block, got %q", res.Severity)
			}
			if res.ConditionKey != "secret_leak" {
				t.Fatalf("expected ConditionKey=secret_leak, got %q", res.ConditionKey)
			}
			if res.Finding == "" {
				t.Fatalf("expected non-empty Finding")
			}
		})
	}
}

func TestSecretLeakDetectorRedactsFinding(t *testing.T) {
	d := newSecretLeakDetector()
	secret := "sk-abcdef0123456789ABCD"
	art := DetectionArtifact{
		DiffText: "+const apiKey = \"" + secret + "\"\n",
	}

	res := d.Detect(context.Background(), art)
	if !res.Detected {
		t.Fatalf("expected Detected=true, got false")
	}
	if strings.Contains(res.Finding, secret) {
		t.Fatalf("Finding leaked the full secret value: %q", res.Finding)
	}
}

func TestSecretLeakDetectorCleanReleases(t *testing.T) {
	d := newSecretLeakDetector()
	art := DetectionArtifact{
		Summary: "Refactor pagination helper",
		Deliverables: []string{
			"apps/control-plane/internal/pagination/pagination.go",
		},
		DiffText: `+func Paginate(items []int, pageSize int) [][]int {
+	var pages [][]int
+	for i := 0; i < len(items); i += pageSize {
+		end := i + pageSize
+		if end > len(items) {
+			end = len(items)
+		}
+		pages = append(pages, items[i:end])
+	}
+	return pages
+}
`,
	}

	res := d.Detect(context.Background(), art)
	if res.Detected {
		t.Fatalf("expected Detected=false for clean diff, got true with Finding=%q", res.Finding)
	}
}

func TestSecretLeakDetectorFiresOnDeliverableOnly(t *testing.T) {
	d := newSecretLeakDetector()
	art := DetectionArtifact{
		Summary: "Add config file",
		Deliverables: []string{
			"AKIAABCDEFGHIJKLMNOP appears only in a deliverable, not the diff",
		},
	}

	res := d.Detect(context.Background(), art)
	if !res.Detected {
		t.Fatalf("expected Detected=true for secret in Deliverables, got false")
	}
	if res.ConditionKey != "secret_leak" {
		t.Fatalf("expected ConditionKey=secret_leak, got %q", res.ConditionKey)
	}
}

func TestSecretLeakDetectorKey(t *testing.T) {
	d := newSecretLeakDetector()
	if d.Key() != "secret_leak" {
		t.Fatalf("expected Key()=secret_leak, got %q", d.Key())
	}
}

var _ ConditionDetector = (*RuleDetector)(nil)
var _ ConditionDetector = (*LLMPromptDetector)(nil)

func TestLLMDetectorParsesDetected(t *testing.T) {
	client := fakeChatCompletionClient{content: `{"detected":true,"finding":"SQL 拼接注入风险"}`}
	d := newCodeReviewDetector(client, "test-model")

	res := d.Detect(context.Background(), DetectionArtifact{
		DiffText: "+query := \"SELECT * FROM users WHERE id = \" + userInput\n",
	})

	if !res.Detected {
		t.Fatalf("expected Detected=true, got false")
	}
	if res.Severity != "major" {
		t.Fatalf("expected Severity=major, got %q", res.Severity)
	}
	if res.ConditionKey != "code_review" {
		t.Fatalf("expected ConditionKey=code_review, got %q", res.ConditionKey)
	}
	if res.Finding != "SQL 拼接注入风险" {
		t.Fatalf("expected Finding to be carried through, got %q", res.Finding)
	}
}

func TestLLMDetectorCleanReleases(t *testing.T) {
	client := fakeChatCompletionClient{content: `{"detected":false}`}
	d := newCodeReviewDetector(client, "test-model")

	res := d.Detect(context.Background(), DetectionArtifact{
		DiffText: "+func Add(a, b int) int { return a + b }\n",
	})

	if res.Detected {
		t.Fatalf("expected Detected=false for clean diff, got true with Finding=%q", res.Finding)
	}
}

func TestLLMDetectorParseFailReleases(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{name: "plain garbage", content: "this is not json at all"},
		{name: "malformed json", content: `{"detected": tru`},
		{name: "detected field absent", content: `{"finding":"noted but no verdict"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeChatCompletionClient{content: tc.content}
			d := newCodeReviewDetector(client, "test-model")

			res := d.Detect(context.Background(), DetectionArtifact{DiffText: "+x := 1\n"})

			if res.Detected {
				t.Fatalf("expected Detected=false (default release) for unparseable/absent reply %q, got true", tc.content)
			}
		})
	}
}

func TestLLMDetectorClientErrorReleases(t *testing.T) {
	client := fakeChatCompletionClient{err: errors.New("upstream unavailable")}
	d := newCodeReviewDetector(client, "test-model")

	res := d.Detect(context.Background(), DetectionArtifact{DiffText: "+x := 1\n"})

	if res.Detected {
		t.Fatalf("expected Detected=false when client errors, got true")
	}
}

func TestLLMDetectorPromptFramedAsViolation(t *testing.T) {
	d := newCodeReviewDetector(fakeChatCompletionClient{content: `{"detected":false}`}, "test-model")

	prompt := d.systemPrompt
	for _, want := range []string{"检测", "违反", "detected"} {
		if !strings.Contains(strings.ToLower(prompt), strings.ToLower(want)) {
			t.Fatalf("expected system prompt to contain detection framing %q, got: %s", want, prompt)
		}
	}
	for _, forbidden := range []string{"打分", "评分", "score", "正确性"} {
		if strings.Contains(strings.ToLower(prompt), strings.ToLower(forbidden)) {
			t.Fatalf("expected system prompt to NOT contain scoring framing %q, got: %s", forbidden, prompt)
		}
	}
}

func TestLLMDetectorKey(t *testing.T) {
	d := newCodeReviewDetector(fakeChatCompletionClient{content: `{"detected":false}`}, "test-model")
	if d.Key() != "code_review" {
		t.Fatalf("expected Key()=code_review, got %q", d.Key())
	}
}
