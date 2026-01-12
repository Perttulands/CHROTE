FROM ollama/ollama:latest

# Configure Ollama
ENV OLLAMA_HOST=0.0.0.0:11434
ENV OLLAMA_MODELS=/root/.ollama/models

# Restrict CORS origins to specific trusted sources
ENV OLLAMA_ORIGINS=http://localhost,http://127.0.0.1,http://arena,https://arena

# Ollama API Port
EXPOSE 11434
