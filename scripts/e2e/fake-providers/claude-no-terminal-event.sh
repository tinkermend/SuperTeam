#!/bin/bash
# 假 provider：发一条合法 claude system 行(带 session_id)，然后 exit 0 且不发 result。
# 对应 PROVIDER_NO_TERMINAL_EVENT（输出格式漂移/空跑的典型形态）。
echo '{"type":"system","subtype":"init","session_id":"ses_e2e_noterminal"}'
exit 0
