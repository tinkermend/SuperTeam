package storage

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

func TestS3ObjectStorePutGetExistsAndDelete(t *testing.T) {
	client := &recordingS3Client{
		getBody:        "stored report",
		getContentType: "text/plain",
	}
	store, err := NewS3ObjectStore(client, "superteam-artifacts")
	if err != nil {
		t.Fatalf("expected object store: %v", err)
	}

	ref, err := store.PutObject(t.Context(), "tasks/42/report.txt", strings.NewReader("stored report"), PutObjectOptions{
		ContentType: "text/plain",
		Metadata: map[string]string{
			"task_id": "42",
		},
	})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	if ref.URI != "s3://superteam-artifacts/tasks/42/report.txt" {
		t.Fatalf("expected stable s3 URI, got %q", ref.URI)
	}
	if client.putInput == nil || aws.ToString(client.putInput.Bucket) != "superteam-artifacts" {
		t.Fatal("expected bucket to be passed to PutObject")
	}
	if aws.ToString(client.putInput.Key) != "tasks/42/report.txt" {
		t.Fatalf("expected object key, got %q", aws.ToString(client.putInput.Key))
	}
	if client.putBody != "stored report" {
		t.Fatalf("expected body to be uploaded, got %q", client.putBody)
	}
	if aws.ToString(client.putInput.ContentType) != "text/plain" {
		t.Fatalf("expected content type, got %q", aws.ToString(client.putInput.ContentType))
	}
	if client.putInput.Metadata["task_id"] != "42" {
		t.Fatalf("expected metadata to be forwarded, got %#v", client.putInput.Metadata)
	}

	object, err := store.GetObject(t.Context(), "tasks/42/report.txt")
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	defer object.Body.Close()
	body, err := io.ReadAll(object.Body)
	if err != nil {
		t.Fatalf("read object body: %v", err)
	}
	if string(body) != "stored report" {
		t.Fatalf("expected downloaded body, got %q", string(body))
	}
	if object.ContentType != "text/plain" {
		t.Fatalf("expected content type, got %q", object.ContentType)
	}

	exists, err := store.Exists(t.Context(), "tasks/42/report.txt")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Fatal("expected object to exist")
	}

	if err := store.DeleteObject(t.Context(), "tasks/42/report.txt"); err != nil {
		t.Fatalf("delete object: %v", err)
	}
	if client.deleteInput == nil || aws.ToString(client.deleteInput.Key) != "tasks/42/report.txt" {
		t.Fatal("expected DeleteObject to receive the object key")
	}
}

func TestS3ObjectStoreExistsReturnsFalseForMissingObject(t *testing.T) {
	store, err := NewS3ObjectStore(&recordingS3Client{
		headErr: apiError{code: "NotFound", message: "missing"},
	}, "superteam-artifacts")
	if err != nil {
		t.Fatalf("expected object store: %v", err)
	}

	exists, err := store.Exists(t.Context(), "missing.txt")
	if err != nil {
		t.Fatalf("expected missing object to be non-fatal: %v", err)
	}
	if exists {
		t.Fatal("expected missing object to return false")
	}
}

func TestS3ObjectStoreRejectsInvalidInput(t *testing.T) {
	if _, err := NewS3ObjectStore(nil, "bucket"); err == nil {
		t.Fatal("expected nil S3 client to fail")
	}
	if _, err := NewS3ObjectStore(&recordingS3Client{}, " "); err == nil {
		t.Fatal("expected blank bucket to fail")
	}

	store, err := NewS3ObjectStore(&recordingS3Client{}, "bucket")
	if err != nil {
		t.Fatalf("expected object store: %v", err)
	}
	if _, err := store.PutObject(t.Context(), "", strings.NewReader("body"), PutObjectOptions{}); err == nil {
		t.Fatal("expected blank key to fail")
	}
	if _, err := store.PutObject(t.Context(), "key", nil, PutObjectOptions{}); err == nil {
		t.Fatal("expected nil body to fail")
	}
	if _, err := store.GetObject(t.Context(), " "); err == nil {
		t.Fatal("expected blank get key to fail")
	}
	if err := store.DeleteObject(t.Context(), " "); err == nil {
		t.Fatal("expected blank delete key to fail")
	}
}

type recordingS3Client struct {
	putInput       *s3.PutObjectInput
	putBody        string
	getInput       *s3.GetObjectInput
	getBody        string
	getContentType string
	headInput      *s3.HeadObjectInput
	headErr        error
	deleteInput    *s3.DeleteObjectInput
}

func (c *recordingS3Client) PutObject(ctx context.Context, input *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	c.putInput = input
	if input.Body != nil {
		body, err := io.ReadAll(input.Body)
		if err != nil {
			return nil, err
		}
		c.putBody = string(body)
	}
	return &s3.PutObjectOutput{}, nil
}

func (c *recordingS3Client) GetObject(ctx context.Context, input *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	c.getInput = input
	return &s3.GetObjectOutput{
		Body:        io.NopCloser(strings.NewReader(c.getBody)),
		ContentType: aws.String(c.getContentType),
	}, nil
}

func (c *recordingS3Client) HeadObject(ctx context.Context, input *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	c.headInput = input
	if c.headErr != nil {
		return nil, c.headErr
	}
	return &s3.HeadObjectOutput{}, nil
}

func (c *recordingS3Client) DeleteObject(ctx context.Context, input *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	c.deleteInput = input
	return &s3.DeleteObjectOutput{}, nil
}

type apiError struct {
	code    string
	message string
}

func (e apiError) Error() string {
	return e.code + ": " + e.message
}

func (e apiError) ErrorCode() string {
	return e.code
}

func (e apiError) ErrorMessage() string {
	return e.message
}

func (e apiError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}

var _ error = apiError{}
var _ smithy.APIError = apiError{}
