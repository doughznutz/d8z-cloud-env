# Use a minimal Python image
FROM python:3.11-slim

# Set working directory
WORKDIR /app

# Copy the script into the image
COPY ollama_proxy.py .

# Install required Python packages
RUN pip install --no-cache-dir flask openai

# Set environment variable for OpenAI key (can be overridden at runtime)
ENV OPENAI_API_KEY=""

# Expose Ollama-compatible port
EXPOSE 11434

# Run the proxy server
CMD ["python", "ollama_proxy.py"]