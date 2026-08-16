package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

var _ BucketProvisioner = (*s3.Client)(nil)

// BucketProvisioner is the Head/Create subset used to idempotently ensure a bucket.
type BucketProvisioner interface {
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
	CreateBucket(ctx context.Context, params *s3.CreateBucketInput, optFns ...func(*s3.Options)) (*s3.CreateBucketOutput, error)
}

// EnsureBucketResult reports whether CreateBucket ran.
type EnsureBucketResult struct {
	Created bool
}

// EnsureBucket makes bucket exist. HeadBucket success is a no-op. Missing
// buckets are created. Already-exists on create is treated as success.
// Prefixes (skills/, artifacts/, runs/) are not created — first PutObject does that.
// The Control Plane process itself never calls this; operators run object-store-init.
func EnsureBucket(ctx context.Context, client BucketProvisioner, bucket, region string) (EnsureBucketResult, error) {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return EnsureBucketResult{}, fmt.Errorf("object store bucket is required")
	}
	if client == nil {
		return EnsureBucketResult{}, fmt.Errorf("s3 client is required")
	}

	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return EnsureBucketResult{}, nil
	}
	if !isBucketMissing(err) {
		return EnsureBucketResult{}, fmt.Errorf("head bucket %q: %w", bucket, err)
	}

	input := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	if constraint := createBucketLocationConstraint(region); constraint != "" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(constraint),
		}
	}
	_, err = client.CreateBucket(ctx, input)
	if err == nil {
		return EnsureBucketResult{Created: true}, nil
	}
	if isBucketAlreadyExists(err) {
		return EnsureBucketResult{}, nil
	}
	return EnsureBucketResult{}, fmt.Errorf("create bucket %q: %w", bucket, err)
}

func createBucketLocationConstraint(region string) string {
	region = strings.TrimSpace(region)
	if region == "" || strings.EqualFold(region, "us-east-1") {
		return ""
	}
	return region
}

func isBucketMissing(err error) bool {
	if s3APIErrorCodeIn(err, "NotFound", "NoSuchBucket", "404", "NoSuchBucketException") {
		return true
	}
	var statusErr interface{ HTTPStatusCode() int }
	return errors.As(err, &statusErr) && statusErr.HTTPStatusCode() == 404
}

func isBucketAlreadyExists(err error) bool {
	return s3APIErrorCodeIn(err, "BucketAlreadyOwnedByYou", "BucketAlreadyExists")
}

func s3APIErrorCodeIn(err error, codes ...string) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	got := apiErr.ErrorCode()
	for _, code := range codes {
		if got == code {
			return true
		}
	}
	return false
}
