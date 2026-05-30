FROM python:3.11-slim

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY main.py .

ENV SERVER_ID="#1"
ENV PORT=9001
ENV GATEWAY_URL="http://localhost:8080"
ENV HEARTBEAT_INTERVAL=3

EXPOSE 9001

CMD ["python", "main.py"]
