#!/bin/bash
COMET_ENV="D:/project/coding-agent/.cursor/skills/comet/scripts/comet-env.sh"
source "$COMET_ENV"

echo "=== Entry Check ==="
"$COMET_STATE" check unified-event-sink design

echo ""
echo "=== Generate Handoff ==="
"$COMET_HANDOFF" unified-event-sink design --write
