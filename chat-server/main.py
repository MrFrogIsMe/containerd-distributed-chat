import os
import threading
import time
from datetime import datetime, timezone

from flask import Flask, jsonify, request
import requests

SERVER_ID = os.environ.get("SERVER_ID", "#1")
if not SERVER_ID.startswith("#"):
    SERVER_ID = f"#{SERVER_ID}"
PORT = int(os.environ.get("PORT", "9001"))
GATEWAY_URL = os.environ.get("GATEWAY_URL", "http://localhost:8080")
HEARTBEAT_INTERVAL = float(os.environ.get("HEARTBEAT_INTERVAL", "3"))
REGISTER_RETRY_INTERVAL = float(os.environ.get("REGISTER_RETRY_INTERVAL", "3"))

app = Flask(__name__)
messages = []
messages_lock = threading.Lock()


def now_iso():
    return datetime.now(timezone.utc).isoformat()


@app.route("/send", methods=["POST"])
def send():
    data = request.get_json(force=True)
    if not data:
        return jsonify({"ok": False, "error": "invalid JSON payload"}), 400

    user = data.get("user")
    message = data.get("message")
    if not isinstance(user, str) or not isinstance(message, str):
        return jsonify({"ok": False, "error": "user and message must be strings"}), 400

    item = {
        "user": user,
        "message": message,
        "ts": now_iso(),
        "server_id": SERVER_ID,
    }
    with messages_lock:
        messages.append(item)

    return jsonify({"ok": True, "server_id": SERVER_ID})


@app.route("/messages", methods=["GET"])
def get_messages():
    with messages_lock:
        return jsonify([m.copy() for m in messages])


@app.route("/health", methods=["GET"])
def health():
    return jsonify({"status": "ok", "server_id": SERVER_ID})


def register_with_gateway():
    url = f"{GATEWAY_URL}/register"
    payload = {"id": SERVER_ID, "addr": f"localhost:{PORT}"}
    headers = {"Content-Type": "application/json"}
    try:
        resp = requests.post(url, json=payload, headers=headers, timeout=5)
        resp.raise_for_status()
        app.logger.info("registered with gateway: %s", payload)
        return True
    except Exception as exc:
        app.logger.warning("register failed: %s", exc)
        return False


def wait_for_registration():
    while True:
        if register_with_gateway():
            return
        app.logger.info("retrying gateway registration in %ss", REGISTER_RETRY_INTERVAL)
        time.sleep(REGISTER_RETRY_INTERVAL)


def send_heartbeat():
    url = f"{GATEWAY_URL}/heartbeat"
    payload = {"id": SERVER_ID}
    headers = {"Content-Type": "application/json"}
    while True:
        try:
            resp = requests.post(url, json=payload, headers=headers, timeout=5)
            resp.raise_for_status()
            app.logger.debug("heartbeat sent: %s", payload)
        except Exception as exc:
            app.logger.warning("heartbeat failed: %s", exc)
        time.sleep(HEARTBEAT_INTERVAL)


if __name__ == "__main__":
    wait_for_registration()
    heartbeat_thread = threading.Thread(target=send_heartbeat, daemon=True)
    heartbeat_thread.start()

    app.run(host="0.0.0.0", port=PORT)
