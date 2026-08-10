#!/bin/bash

# Runtime 每次派发前用 `<bin> --version`（3 秒超时）探活；不答就会被判
# provider_unavailable，预检闸直接拦下派发，任务卡在 waiting_human/attempts=0，
# 看起来像"审批不放行"。踩过一次，别删这段。
if [ "$1" = "--version" ]; then
  echo "fake-provider 0.0.0 (superteam e2e)"
  exit 0
fi
echo '{"type":"system","subtype":"init","session_id":"ses_e2e_rl_review"}'
echo "Error: rate limit exceeded" >&2
exit 1
