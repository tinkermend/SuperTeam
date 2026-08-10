#!/bin/bash
echo '{"type":"system","subtype":"init","session_id":"ses_e2e_rl_review"}'
echo "Error: rate limit exceeded" >&2
exit 1
