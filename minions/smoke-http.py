#!/usr/bin/env python3
"""Probe the built HTTP scaffold and real SIGTERM/SIGINT shutdown locally."""

import argparse
import json
import os
import selectors
import signal
import subprocess
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent


def probe(binary, stop_signal):
    env = os.environ.copy()
    env.update(SLOVNYK_HTTP_ADDR="127.0.0.1:0", SLOVNYK_LOG_LEVEL="info", SLOVNYK_SHUTDOWN_TIMEOUT="2s")
    process = subprocess.Popen([str(binary), "serve"], cwd=ROOT, env=env,
                               stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(process.stderr, selectors.EVENT_READ)
            if not selector.select(5):
                raise RuntimeError("HTTP startup timeout")
            first = process.stderr.readline()
        startup = json.loads(first)
        if startup.get("event") != "http_started":
            raise RuntimeError("Missing HTTP startup event")
        checks = []
        # Disable proxies: this minion only addresses its own loopback process.
        client = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for method, path, expected in [("GET", "/healthz?q=synthetic-secret", 200),
                                        ("HEAD", "/healthz", 200), ("GET", "/readyz", 503)]:
            request = urllib.request.Request("http://" + startup["address"] + path, method=method)
            try:
                response = client.open(request, timeout=3)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                body = response.read()
                if response.status != expected:
                    raise RuntimeError(f"{method} {path.split('?')[0]}: unexpected HTTP status {response.status}")
                if method == "HEAD" and body:
                    raise RuntimeError("HEAD response has a body")
                if path == "/readyz" and json.loads(body).get("reason") != "storage_not_initialized":
                    raise RuntimeError("Unexpected scaffold readiness reason")
                checks.append({"method": method, "path": path.split("?")[0], "status": response.status})
        process.send_signal(stop_signal)
        _, tail = process.communicate(timeout=5)
        logs = first + tail
        events = [json.loads(line).get("event") for line in logs.splitlines()]
        if process.returncode != 0 or events != ["http_started", "http_draining", "http_stopped"]:
            raise RuntimeError("Process shutdown or lifecycle events failed")
        if "synthetic-secret" in logs or "q=" in logs:
            raise RuntimeError("Query text appeared in process logs")
        return {"signal": stop_signal.name, "exit_code": process.returncode, "http": checks,
                "events": events, "query_text_not_logged": True}
    finally:
        if process.poll() is None:
            process.kill()
            process.communicate(timeout=5)
        if process.stderr:
            process.stderr.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, default=ROOT / "bin/slovnyk")
    args = parser.parse_args()
    binary = args.binary.resolve()
    report = {"passed": False, "results": []}
    try:
        for stop_signal in [signal.SIGTERM, signal.SIGINT]:
            report["results"].append(probe(binary, stop_signal))
        report["passed"] = True
    except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
        # Do not echo arbitrary subprocess output or runtime configuration.
        report["error"] = type(error).__name__
    output = ROOT / "tmp/minions/http-smoke.json"
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(report))
    return int(not report["passed"])


if __name__ == "__main__":
    raise SystemExit(main())
