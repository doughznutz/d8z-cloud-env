#!/bin/bash

# This script sends a test query to the LiteLLM proxy emulating a native
# Google Gemini API request. This is to test if LiteLLM can handle
# requests from clients like geminicli.
#
# Make sure the docker containers are running, e.g. `docker compose up`
#
# Usage:
# 1. Make the script executable:
#    chmod +x dev/litellm/test_gemini_native.sh
# 2. Run the script:
#    ./dev/litellm/test_gemini_native.sh

# Note: The API key in the URL can be any string when sending to LiteLLM,
# as LiteLLM uses the server-side key from its config.
# We add it here to more closely emulate a real client call.
curl "http://localhost:4000/v1beta/models/gemini-2.5-pro:generateContent?key=ANY_KEY_WILL_DO" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [
      {
        "parts": [
          {
            "text": "This is a test query using the native Gemini endpoint."
          }
        ]
      }
    ]
  }'
