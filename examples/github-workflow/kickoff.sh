#!/usr/bin/env bash
# Kick off a run of the workflow created by this example.
#
# Reads url / api_key / workspace from terraform.tfvars, pulls the workflow ID
# from Terraform state, resolves the workspace slug to its UUID, then triggers a
# run via the Ctrlplane Connect RPC (WorkflowService/CreateWorkflowRun).
#
# Usage:
#   ./kickoff.sh [NAME]
#
#   NAME             value for the workflow's required `name` input (default: kickoff-demo)
#   WANDB_VERSION    value for the `wandb_version` input (default: 0.80.0)
set -euo pipefail

cd "$(dirname "$0")"

for bin in curl jq terraform; do
  command -v "$bin" >/dev/null 2>&1 || { echo "error: '$bin' is required" >&2; exit 1; }
done

# --- read provider config from terraform.tfvars ---
tfvar() { sed -n "s/^[[:space:]]*$1[[:space:]]*=[[:space:]]*\"\([^\"]*\)\".*/\1/p" terraform.tfvars; }
URL=$(tfvar url)
API_KEY=$(tfvar api_key)
WORKSPACE=$(tfvar workspace)
BASE=${URL%/}

[ -n "$URL" ]       || { echo "error: 'url' not found in terraform.tfvars" >&2; exit 1; }
[ -n "$API_KEY" ]   || { echo "error: 'api_key' not found in terraform.tfvars" >&2; exit 1; }
[ -n "$WORKSPACE" ] || { echo "error: 'workspace' not found in terraform.tfvars" >&2; exit 1; }

# --- inputs ---
NAME=${1:-kickoff-demo}
WANDB_VERSION=${WANDB_VERSION:-0.80.0}

post() { # post <rpc-path> <json-body>
  curl -sS -X POST "${BASE}/$1" \
    -H "Content-Type: application/json" \
    -H "X-API-Key: ${API_KEY}" \
    -d "$2"
}

# --- workflow ID from Terraform state ---
WORKFLOW_ID=$(terraform state pull \
  | jq -r '.resources[] | select(.type=="ctrlplane_workflow") | .instances[0].attributes.id' \
  | head -n1)
[ -n "$WORKFLOW_ID" ] && [ "$WORKFLOW_ID" != "null" ] \
  || { echo "error: no ctrlplane_workflow found in state — run 'terraform apply' first" >&2; exit 1; }

# --- resolve workspace slug -> UUID (skip if already a UUID) ---
if [[ "$WORKSPACE" =~ ^[0-9a-fA-F-]{36}$ ]]; then
  WORKSPACE_ID="$WORKSPACE"
else
  WORKSPACE_ID=$(post "ctrlplane.api.v1.WorkspaceService/GetWorkspaceBySlug" \
    "$(jq -nc --arg slug "$WORKSPACE" '{slug:$slug}')" | jq -r '.id')
fi
[ -n "$WORKSPACE_ID" ] && [ "$WORKSPACE_ID" != "null" ] \
  || { echo "error: could not resolve workspace '$WORKSPACE'" >&2; exit 1; }

# --- fire the run ---
BODY=$(jq -nc \
  --arg ws "$WORKSPACE_ID" \
  --arg wf "$WORKFLOW_ID" \
  --arg name "$NAME" \
  --arg ver "$WANDB_VERSION" \
  '{workspaceId:$ws, workflowId:$wf, inputs:{name:$name, wandb_version:$ver}}')

echo "Triggering workflow $WORKFLOW_ID (name=$NAME, wandb_version=$WANDB_VERSION)..." >&2
post "ctrlplane.api.v1.WorkflowService/CreateWorkflowRun" "$BODY" | jq .
