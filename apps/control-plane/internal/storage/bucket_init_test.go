package storage

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type recordingBucketClient struct {
	headErr   error
	createErr error
	headN     int
	createN   int
	createIn  *s3.CreateBucketInput
}

func (c *recordingBucketClient) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	c.headN++
	if c.headErr != nil {
		return nil, c.headErr
	}
	return &s3.HeadBucketOutput{}, nil
}

func (c *recordingBucketClient) CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error) {
	c.createN++
	c.createIn = params
	if c.createErr != nil {
		return nil, c.createErr
	}
	return &s3.CreateBucketOutput{}, nil
}

func TestEnsureBucketNoopWhenPresent(t *testing.T) {
	client := &recordingBucketClient{}
	got, err := EnsureBucket(t.Context(), client, "superteam-artifacts", "us-east-1")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got.Created || client.createN != 0 || client.headN != 1 {
		t.Fatalf("expected head-only, got %+v head=%d create=%d", got, client.headN, client.createN)
	}
}

func TestEnsureBucketCreatesWhenMissing(t *testing.T) {
	client := &recordingBucketClient{headErr: apiError{code: "NotFound", message: "missing"}}
	got, err := EnsureBucket(t.Context(), client, "superteam-artifacts", "us-east-1")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !got.Created || client.createN != 1 {
		t.Fatalf("expected create, got %+v create=%d", got, client.createN)
	}
	if client.createIn == nil || aws.ToString(client.createIn.Bucket) != "superteam-artifacts" {
		t.Fatal("expected create to target the configured bucket")
	}
	if client.createIn.CreateBucketConfiguration != nil {
		t.Fatal("us-east-1 must not send LocationConstraint")
	}
}

func TestEnsureBucketLocationConstraintForNonUSEast1(t *testing.T) {
	client := &recordingBucketClient{headErr: apiError{code: "NoSuchBucket", message: "missing"}}
	if _, err := EnsureBucket(t.Context(), client, "superteam-artifacts", "cn-guangzhou"); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if client.createIn == nil || client.createIn.CreateBucketConfiguration == nil {
		t.Fatal("expected location constraint")
	}
	if string(client.createIn.CreateBucketConfiguration.LocationConstraint) != "cn-guangzhou" {
		t.Fatalf("constraint=%q", client.createIn.CreateBucketConfiguration.LocationConstraint)
	}
}

func TestEnsureBucketCreateAlreadyExistsIsOK(t *testing.T) {
	client := &recordingBucketClient{
		headErr:   apiError{code: "NotFound", message: "missing"},
		createErr: apiError{code: "BucketAlreadyOwnedByYou", message: "owned"},
	}
	got, err := EnsureBucket(t.Context(), client, "superteam-artifacts", "us-east-1")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if got.Created {
		t.Fatal("already-owned should not report created")
	}
}

func TestEnsureBucketRejectsForbiddenHead(t *testing.T) {
	client := &recordingBucketClient{headErr: apiError{code: "Forbidden", message: "denied"}}
	if _, err := EnsureBucket(t.Context(), client, "superteam-artifacts", "us-east-1"); err == nil {
		t.Fatal("expected forbidden head to fail rather than create")
	}
	if client.createN != 0 {
		t.Fatal("must not create after forbidden head")
	}
}

func TestEnsureBucketRequiresName(t *testing.T) {
	if _, err := EnsureBucket(t.Context(), &recordingBucketClient{}, "  ", "us-east-1"); err == nil {
		t.Fatal("expected blank bucket to fail")
	}
}

type httpStatusErr struct {
	status  int
	message string
}

func (e httpStatusErr) Error() string       { return e.message }
func (e httpStatusErr) HTTPStatusCode() int { return e.status }

func TestEnsureBucketCreatesOnHTTP404(t *testing.T) {
	client := &recordingBucketClient{headErr: httpStatusErr{status: 404, message: "not found"}}
	got, err := EnsureBucket(t.Context(), client, "superteam-artifacts", "us-east-1")
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !got.Created {
		t.Fatal("expected create after HTTP 404 head")
	}
}

func TestEnsureBucketRejectsHTTP403(t *testing.T) {
	client := &recordingBucketClient{headErr: httpStatusErr{status: 403, message: "denied"}}
	if _, err := EnsureBucket(t.Context(), client, "superteam-artifacts", "us-east-1"); err == nil {
		t.Fatal("expected HTTP 403 head to fail rather than create")
	}
	if client.createN != 0 {
		t.Fatal("must not create after HTTP 403 head")
	}
}
