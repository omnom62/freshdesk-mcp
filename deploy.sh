#!/bin/bash
set -e

IMAGE="australia-southeast1-docker.pkg.dev/wb-sandbox-1/freshdesk-mcp/freshdesk-mcp"

echo "Building..."
docker build --no-cache -t ${IMAGE}:latest .

echo "Pushing..."
DIGEST=$(docker push ${IMAGE}:latest | grep "digest:" | awk '{print $3}')
echo "Digest: $DIGEST"

echo "Deploying to Cloud Run..."
gcloud run deploy freshdesk-mcp \
  --image ${IMAGE}@${DIGEST} \
  --region australia-southeast1 \
  --platform managed \
  --allow-unauthenticated \
  --set-secrets FRESHDESK_DOMAIN=FRESHDESK_DOMAIN:latest,FRESHDESK_API_KEY=FRESHDESK_API_KEY:latest,MCP_TOKEN=MCP_TOKEN:latest \
  --set-env-vars GCP_VISION_PROJECT=wb-sandbox-1 \
  --project wb-sandbox-1

echo "Done."
