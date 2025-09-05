# Ollama Proxy

This project is a Go-based proxy server that provides an OpenAI-compatible API endpoint for interacting with various Large Language Models (LLMs). It acts as a translation layer, accepting requests in the OpenAI chat completions format and routing them to different backend providers such as Google Gemini, OpenAI, and Groq.

## Features

- **OpenAI-Compatible API**: Exposes a `/v1/chat/completions` endpoint, allowing it to be used as a drop-in replacement for the OpenAI API.
- **Multi-Provider Support**: Routes requests to different LLM providers based on the `model` specified in the request. Supported providers include:
  - Google Gemini (`gemini-2.0-flash`)
  - OpenAI (`gpt-4.1`)
  - Groq (`llama-3.3-70b-versatile`)
- **Request and Response Translation**: Translates OpenAI-formatted requests to the native format of the backend provider and converts the responses back to the OpenAI format.
- **Streaming Support**: Supports streaming responses from the backend providers to the client in real-time.
- **Database Logging**: Logs all requests and responses to a PostgreSQL database for monitoring and analysis.
- **Dockerized**: The application is containerized using Docker for easy deployment and scalability.

## How it Works

The proxy server listens for incoming requests on port `11434`. When a request is received at the `/v1/chat/completions` endpoint, the server inspects the `model` field in the request body to determine which backend provider to use.

- If the model is `gemini-2.0-flash`, the request is translated to the Gemini API format and sent to the Google Gemini API. The response is then converted back to the OpenAI streaming format and sent to the client.
- For other models, the request is forwarded to the corresponding provider's API (OpenAI or Groq).

All interactions are logged to a PostgreSQL database named `ollama_logs` for auditing and debugging purposes.

The server also implements other Ollama-compatible endpoints, such as `/api/tags` and `/api/show`, to provide information about the available models.

## Getting Started

To run this proxy server, you will need to have Docker and Docker Compose installed.

1. **Clone the repository:**

   ```bash
   git clone https://github.com/your-username/ollama-proxy.git
   cd ollama-proxy
   ```

2. **Set up the environment variables:**

   Create a `.env` file in the root of the project and add the following environment variables:

   ```
   OPENAI_API_KEY=your-openai-api-key
   GEMINI_API_KEY=your-gemini-api-key
   GROQ_API_KEY=your-groq-api-key
   POSTGRES_PASSWORD=your-postgres-password
   ```

3. **Build and run the Docker container:**

   ```bash
   docker-compose up -d
   ```

The proxy server will now be running on port `11434`.

## API Endpoints

- `POST /v1/chat/completions`: The main endpoint for chat completions.
- `GET /api/tags`: Lists the available models.
- `GET /api/show`: Provides details about a specific model.

## Testing

The `test` directory contains a sample Go client and `curl` scripts that can be used to test the proxy server.

### Example `curl` request:

```bash
curl http://localhost:11434/v1/chat/completions \
  -X POST \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.0-flash",
    "messages": [
      {
        "role": "user",
        "content": "What is the capital of France?"
      }
    ],
    "temperature": 0.7,
    "stream": false
  }'
```
