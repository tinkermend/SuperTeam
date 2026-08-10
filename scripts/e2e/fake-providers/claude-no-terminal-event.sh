#!/bin/bash

# Runtime 每次派发前用 `<bin> --version`（3 秒超时）探活；不答就会被判
# provider_unavailable，预检闸直接拦下派发，任务卡在 waiting_human/attempts=0，
# 看起来像"审批不放行"。踩过一次，别删这段。
if [ "$1" = "--version" ]; then
  echo "fake-provider 0.0.0 (superteam e2e)"
  exit 0
fi
# 假 provider：发一条合法 claude system 行(带 session_id)，然后 exit 0 且不发 result。
# 对应 PROVIDER_NO_TERMINAL_EVENT（输出格式漂移/空跑的典型形态）。
echo '{"type":"system","subtype":"init","session_id":"ses_e2e_noterminal"}'
exit 0
