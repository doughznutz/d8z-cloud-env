FROM python:3.12-slim

RUN pip install --no-cache-dir streamlit requests docker

WORKDIR /app
COPY app.py /app/app.py
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Streamlit dashboard.
EXPOSE 8502

ENTRYPOINT ["/app/entrypoint.sh"]
