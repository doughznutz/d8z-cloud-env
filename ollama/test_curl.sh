curl http://ollama:11434/v1/chat/completions \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-3.5-turbo",
    "messages": [
      {
        "role": "user",
        "content": "What is the capital of France?"
      }
    ],
    "temperature": 0.7,
    "stream": false
  }'

curl http://ollama:11434/v1/chat/completions \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.0-flash",
    "messages": [
      {
        "role": "system",
        "content": "What is the capital of France?"
      }
    ],
    "temperature": 0.7,
    "stream": false
  }'
