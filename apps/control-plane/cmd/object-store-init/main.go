// object-store-init 读取 Control Plane 同一份 objectStore 配置（yaml + S3_* 覆盖），
// 幂等建桶并写入控制台预览所需 CORS。控制面进程启动不会自动建桶。
//
//	go run ./apps/control-plane/cmd/object-store-init --config apps/control-plane/config/config.yaml
//	go run ./apps/control-plane/cmd/object-store-init --config ... --check
//	go run ./apps/control-plane/cmd/object-store-init --config ... --skip-cors
//
// 环境变量：
//
//	BUCKET_CORS_ORIGINS  逗号分隔允许来源；默认 http://127.0.0.1:3100,http://localhost:3100
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/superteam/control-plane/internal/config"
	"github.com/superteam/control-plane/internal/storage"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "object-store-init: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("object-store-init", flag.ContinueOnError)
	configPath := fs.String("config", "apps/control-plane/config/config.yaml", "path to control-plane config.yaml")
	checkOnly := fs.Bool("check", false, "only verify bucket exists and CORS covers origins; do not create or write")
	skipCORS := fs.Bool("skip-cors", false, "ensure bucket only; do not write CORS")
	originsFlag := fs.String("origins", "", "comma-separated allowed origins (overrides BUCKET_CORS_ORIGINS)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	warnEnvOverlay()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client, err := storage.NewS3Client(ctx, storage.ObjectStoreConfig{
		Endpoint:        cfg.ObjectStore.Endpoint,
		Region:          cfg.ObjectStore.Region,
		Bucket:          cfg.ObjectStore.Bucket,
		AccessKeyID:     cfg.ObjectStore.AccessKeyID,
		SecretAccessKey: cfg.ObjectStore.SecretAccessKey,
		ForcePathStyle:  cfg.ObjectStore.ForcePathStyle,
	})
	if err != nil {
		return fmt.Errorf("s3 client: %w", err)
	}

	bucket := strings.TrimSpace(cfg.ObjectStore.Bucket)
	fmt.Printf("endpoint=%s bucket=%s region=%s forcePathStyle=%v\n",
		cfg.ObjectStore.Endpoint, bucket, cfg.ObjectStore.Region, cfg.ObjectStore.ForcePathStyle)

	if *checkOnly {
		if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)}); err != nil {
			return fmt.Errorf("head bucket %q: %w", bucket, err)
		}
		fmt.Println("bucket: exists")
		if *skipCORS {
			return nil
		}
		origins := resolveOrigins(*originsFlag)
		return checkCORS(ctx, client, bucket, origins)
	}

	result, err := storage.EnsureBucket(ctx, client, bucket, cfg.ObjectStore.Region)
	if err != nil {
		return err
	}
	if result.Created {
		fmt.Println("bucket: created")
	} else {
		fmt.Println("bucket: exists")
	}

	if *skipCORS {
		return nil
	}
	origins := resolveOrigins(*originsFlag)
	if len(origins) == 0 {
		return errors.New("no origins: set --origins or BUCKET_CORS_ORIGINS")
	}
	fmt.Printf("desired_origins=%s\n", strings.Join(origins, ","))
	return applyCORS(ctx, client, bucket, origins)
}

func warnEnvOverlay() {
	var keys []string
	for _, key := range []string{
		"S3_ENDPOINT", "S3_REGION", "S3_BUCKET", "S3_FORCE_PATH_STYLE",
		"S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY",
	} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "note: %s in the environment override config.yaml objectStore\n", strings.Join(keys, ", "))
}

func resolveOrigins(flagValue string) []string {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("BUCKET_CORS_ORIGINS"))
	}
	if raw == "" {
		raw = "http://127.0.0.1:3100,http://localhost:3100"
	}
	return splitCSV(raw)
}

func applyCORS(ctx context.Context, client *s3.Client, bucket string, origins []string) error {
	_, err := client.PutBucketCors(ctx, &s3.PutBucketCorsInput{
		Bucket: aws.String(bucket),
		CORSConfiguration: &types.CORSConfiguration{
			CORSRules: []types.CORSRule{desiredRule(origins)},
		},
	})
	if err != nil {
		return fmt.Errorf("PutBucketCors: %w", err)
	}
	fmt.Println("cors: applied")
	return checkCORS(ctx, client, bucket, origins)
}

func checkCORS(ctx context.Context, client *s3.Client, bucket string, desired []string) error {
	out, err := client.GetBucketCors(ctx, &s3.GetBucketCorsInput{Bucket: aws.String(bucket)})
	if err != nil {
		return fmt.Errorf("GetBucketCors: %w", err)
	}
	if len(out.CORSRules) == 0 {
		return errors.New("bucket has no CORS rules")
	}
	for i, rule := range out.CORSRules {
		fmt.Printf("cors rule[%d] origins=%v methods=%v\n", i, rule.AllowedOrigins, rule.AllowedMethods)
	}
	if !ruleCoversOrigins(out.CORSRules, desired) {
		return fmt.Errorf("current CORS does not cover all desired origins %v", desired)
	}
	fmt.Println("cors: ok")
	return nil
}

func desiredRule(origins []string) types.CORSRule {
	return types.CORSRule{
		AllowedOrigins: origins,
		AllowedMethods: []string{"GET", "HEAD"},
		AllowedHeaders: []string{"*"},
		ExposeHeaders:  []string{"ETag", "Content-Type", "Content-Length"},
		MaxAgeSeconds:  aws.Int32(3600),
	}
}

func ruleCoversOrigins(rules []types.CORSRule, desired []string) bool {
	have := map[string]struct{}{}
	for _, rule := range rules {
		for _, o := range rule.AllowedOrigins {
			have[o] = struct{}{}
			if o == "*" {
				return true
			}
		}
	}
	for _, o := range desired {
		if _, ok := have[o]; !ok {
			return false
		}
	}
	return true
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
