#!/bin/bash

# This script sends a test query to the LiteLLM proxy to test the
# gemini-1.5-flash model configuration.
#
# Make sure the docker containers are running, e.g. `docker compose up`
#
# Usage:
# 1. Make the script executable:
#    chmod +x dev/litellm/test_query.sh
# 2. Run the script:
#    ./dev/litellm/test_query.sh

curl http://localhost:4000/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-1.5-flash",
    "messages": [
      {
        "role": "user",
        "content": "This is a test query to see if the data shows up in the database."
      }
    ],
    "stream": false
  }'

