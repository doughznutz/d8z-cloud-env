curl http://localhost:11434/api/chat \
  -X POST \
  -H "Content-Type: application/json" \
  -N \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "Can you tell me a joke?"
      }
    ],
    "temperature": 0.7,
    "stream": true
  }'