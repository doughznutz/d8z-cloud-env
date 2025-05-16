from flask import Flask, request, Response, jsonify
import openai
import os

app = Flask(__name__)

# Set your OpenAI key in the environment or directly here (not recommended)
openai.api_key = os.getenv("OPENAI_API_KEY")

@app.route("/api/chat", methods=["POST"])
def proxy_chat():
    data = request.get_json()

    if not data or "messages" not in data:
        return jsonify({"error": "Missing messages"}), 400

    # Translate Ollama request to OpenAI format
    messages = data["messages"]
    model = data.get("model", "gpt-3.5-turbo")
    temperature = data.get("temperature", 0.7)
    stream = data.get("stream", False)

    try:
        if stream:
            def generate():
                response = openai.ChatCompletion.create(
                    model=model,
                    messages=messages,
                    temperature=temperature,
                    stream=True,
                )
                for chunk in response:
                    if "choices" in chunk and chunk["choices"]:
                        delta = chunk["choices"][0]["delta"].get("content", "")
                        if delta:
                            yield delta
            return Response(generate(), content_type="text/plain")

        else:
            response = openai.ChatCompletion.create(
                model=model,
                messages=messages,
                temperature=temperature,
            )
            content = response["choices"][0]["message"]["content"]
            return jsonify({
                "message": {
                    "role": "assistant",
                    "content": content
                },
                "created_at": response["created"],
                "model": model,
            })

    except openai.error.OpenAIError as e:
        return jsonify({"error": str(e)}), 500

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=11434)