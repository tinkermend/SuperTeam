// bucket-cors 幂等写入/检查对象存储桶 CORS（复用 Control Plane objectStore 配置）。
//
// 背景：Console 预览工件时对 presigned GET 发起跨域请求，桶须放行 Web origin。
// 规则模板见 docs/superpowers/specs/2026-07-19-execution-output-attachments-followups.md §2。
//
// 用法：
//
//	go run ./apps/control-plane/cmd/bucket-cors --config apps/control-plane/config/config.yaml
//	go run ./apps/control-plane/cmd/bucket-cors --config ... --check
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
		fmt.Fprintf(os.Stderr, "bucket-cors: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("bucket-cors", flag.ContinueOnError)
	configPath := fs.String("config", "apps/control-plane/config/config.yaml", "path to control-plane config.yaml")
	checkOnly := fs.Bool("check", false, "only read and print current CORS; do not apply")
	originsFlag := fs.String("origins", "", "comma-separated allowed origins (overrides BUCKET_CORS_ORIGINS)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.LoadFromFile(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	origins := resolveOrigins(*originsFlag)
	if len(origins) == 0 {
		return errors.New("no origins: set --origins or BUCKET_CORS_ORIGINS")
	}

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
	fmt.Printf("endpoint=%s bucket=%s forcePathStyle=%v\n", cfg.ObjectStore.Endpoint, bucket, cfg.ObjectStore.ForcePathStyle)
	fmt.Printf("desired_origins=%s\n", strings.Join(origins, ","))

	if *checkOnly {
		return checkCORS(ctx, client, bucket, origins)
	}
	return applyCORS(ctx, client, bucket, origins)
}

func resolveOrigins(flagValue string) []string {
	raw := strings.TrimSpace(flagValue)
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("BUCKET_CORS_ORIGINS"))
	}
	if raw == "" {
		// 与 apps/web/vite.config.ts DEV_SERVER_PORT 及 CP dev CORS 默认一致
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
	fmt.Println("applied: ok")
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
		fmt.Printf("rule[%d] origins=%v methods=%v headers=%v expose=%v maxAge=%v\n",
			i, rule.AllowedOrigins, rule.AllowedMethods, rule.AllowedHeaders, rule.ExposeHeaders, aws.ToInt32(rule.MaxAgeSeconds))
	}
	if !ruleCoversOrigins(out.CORSRules, desired) {
		return fmt.Errorf("current CORS does not cover all desired origins %v", desired)
	}
	fmt.Println("check: ok")
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
