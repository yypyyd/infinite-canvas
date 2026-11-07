"""Inspect/configure only the infinite-canvas production ingress; never print secrets."""

import json
import re
import shutil
import subprocess
import sys
from pathlib import Path

ENV_FILE = Path("/opt/infinite-canvas/.env")
NGINX = shutil.which("nginx") or "/www/server/nginx/sbin/nginx"


def inspect():
    result = subprocess.run([NGINX, "-T"], capture_output=True, text=True, check=True)
    sections = re.split(r"^# configuration file (.+):\s*$", result.stdout, flags=re.M)
    files = []
    for index in range(1, len(sections), 2):
        path, body = sections[index:index + 2]
        if re.search(r"proxy_pass\s+http://(?:127\.0\.0\.1|localhost):3050(?:/)?\s*;", body):
            files.append(path)
            relevant = [line.strip() for line in body.splitlines() if re.search(r"server_name|proxy_pass|proxy_set_header|real_ip|include|location", line)]
            relevant = [re.sub(r"(proxy_set_header\s+(?!Host\b|X-Real-IP\b|X-Forwarded-For\b|X-Forwarded-Proto\b|Upgrade\b|Connection\b)\S+).*;", r"\1 [REDACTED];", line) for line in relevant]
            print(json.dumps({"file": path, "directives": relevant}))
    values = dict(line.split("=", 1) for line in ENV_FILE.read_text().splitlines() if "=" in line and not line.lstrip().startswith("#"))
    print(json.dumps({"configured": {name: bool(values.get(name, "").strip()) for name in ("TURNSTILE_SITE_KEY", "TURNSTILE_SECRET_KEY", "TURNSTILE_HOSTNAMES", "REVERSE_PROXY_TOKEN")}}))
    return files


if __name__ == "__main__":
    if sys.argv[1] != "inspect":
        raise SystemExit("Configuration requires inspected ingress files")
    inspect()
