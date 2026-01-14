#!/bin/bash
# Test script to simulate Gastown sessions
# Run this inside the container: docker exec -it agentarena-dev bash /code/AgentArena/test-sessions.sh
# Or from within tmux in the container: bash /code/AgentArena/test-sessions.sh

export TMUX_TMPDIR=/tmp

echo "Creating test sessions..."

# Create HQ sessions
tmux new-session -d -s "hq-test1" && echo "Created: hq-test1" || echo "Already exists: hq-test1"
tmux new-session -d -s "hq-test2" && echo "Created: hq-test2" || echo "Already exists: hq-test2"

# Create Gastown rig sessions (gt-rigname-agentname format)
tmux new-session -d -s "gt-test1-agent1" && echo "Created: gt-test1-agent1" || echo "Already exists: gt-test1-agent1"
tmux new-session -d -s "gt-test1-agent2" && echo "Created: gt-test1-agent2" || echo "Already exists: gt-test1-agent2"
tmux new-session -d -s "gt-test2-worker" && echo "Created: gt-test2-worker" || echo "Already exists: gt-test2-worker"

echo ""
echo "Current sessions:"
tmux list-sessions

echo ""
echo "Test complete! Refresh the dashboard to see the sessions."
