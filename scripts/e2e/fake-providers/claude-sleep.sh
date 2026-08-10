#!/bin/bash
# 假 provider：发 session 行后长睡，触发墙钟预算熔断（BUDGET_FUSE）。
echo '{"type":"system","subtype":"init","session_id":"ses_e2e_budget"}'
sleep 300
