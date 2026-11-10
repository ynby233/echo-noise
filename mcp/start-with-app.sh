#!/bin/sh
# The application owns container health and exit status. MCP is optional.
# Do not use set -e: an addon failure must never terminate the application.
mcp_pid=
app_pid=

forward_stop() {
  [ -z "$app_pid" ] || kill -TERM "$app_pid" 2>/dev/null || true
  [ -z "$mcp_pid" ] || kill -TERM "$mcp_pid" 2>/dev/null || true
}
trap forward_stop TERM INT

# stdio is started on demand by the AI client with docker exec, not as a daemon.
if [ "${MCP_ENABLED:-true}" != false ] && [ "${MCP_ENABLED:-true}" != 0 ] && [ "${MCP_TRANSPORT:-stdio}" = http ]; then
  node /app/mcp/server.js </dev/null &
  mcp_pid=$!
fi

"$@" &
app_pid=$!
while :; do
  wait "$app_pid"
  app_status=$?
  # wait may have been interrupted by a signal; let the app finish shutdown.
  kill -0 "$app_pid" 2>/dev/null || break
done
if [ -n "$mcp_pid" ]; then
  kill -TERM "$mcp_pid" 2>/dev/null || true
  # Give in-flight writes a bounded grace period; a stuck addon cannot hang shutdown.
  (
    sleep 6
    kill -KILL "$mcp_pid" 2>/dev/null || true
  ) &
  deadline_pid=$!
  while kill -0 "$mcp_pid" 2>/dev/null; do
    wait "$mcp_pid" 2>/dev/null || true
  done
  kill "$deadline_pid" 2>/dev/null || true
  wait "$deadline_pid" 2>/dev/null || true
fi
exit "$app_status"
