"""Inspect/configure only the infinite-canvas production ingress; never print secrets."""

import json
import ipaddress
import os
import re
import secrets
import subprocess
import sys
import tempfile
import urllib.request
from pathlib import Path

ENV_FILE = Path("/opt/infinite-canvas/.env")
NGINX = "/www/server/nginx/sbin/nginx"
SNIPPET = Path("/opt/infinite-canvas/nginx-security.conf")
PROXY_FILES = [
    Path("/www/server/panel/vhost/nginx/canvas.xinghengcode.shop.conf"),
    Path("/www/server/panel/vhost/nginx/proxy/huantu.xyz/00-static-cache_huantu.xyz.conf"),
    Path("/www/server/panel/vhost/nginx/proxy/huantu.xyz/e2429145d566ede34558219db6fe1391_huantu.xyz.conf"),
]


def inspect():
    result = subprocess.run([NGINX, "-T"], capture_output=True, text=True, check=True)
    output = result.stdout + result.stderr
    sections = re.split(r"^# configuration file (.+):[ \t]*$", output, flags=re.M)
    files = []
    for index in range(1, len(sections), 2):
        path, body = sections[index:index + 2]
        if "3050" in body or "huantu.xyz" in body or "canvas.xinghengcode.shop" in body:
            files.append(path)
            relevant = [line.strip() for line in body.splitlines() if re.search(r"server_name|proxy_pass|proxy_set_header|real_ip|include|location", line)]
            relevant = [re.sub(r"(proxy_set_header\s+(?!Host\b|X-Real-IP\b|X-Forwarded-For\b|X-Forwarded-Proto\b|Upgrade\b|Connection\b)\S+).*;", r"\1 [REDACTED];", line) for line in relevant]
            print(json.dumps({"file": path, "directives": relevant}))
    values = dict(line.split("=", 1) for line in ENV_FILE.read_text().splitlines() if "=" in line and not line.lstrip().startswith("#"))
    print(json.dumps({"configured": {name: bool(values.get(name, "").strip()) for name in ("TURNSTILE_SITE_KEY", "TURNSTILE_SECRET_KEY", "TURNSTILE_HOSTNAMES", "REVERSE_PROXY_TOKEN")}}))
    return files


def atomic_write(path, data, mode):
    fd, temporary = tempfile.mkstemp(prefix="." + path.name + ".security-", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
        os.chmod(temporary, mode)
        if path.exists():
            info = path.stat()
            os.chown(temporary, info.st_uid, info.st_gid)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def configure(input_path):
    sys.path.insert(0, str(next(Path(__file__).parent.glob("crossplane-*.whl"))))
    import crossplane

    settings = json.loads(Path(input_path).read_text())
    for name in ("TURNSTILE_SITE_KEY", "TURNSTILE_SECRET_KEY"):
        if not re.fullmatch(r"[A-Za-z0-9_-]{20,}", settings.get(name, "")):
            raise ValueError("Missing or invalid " + name)
    if settings.get("TURNSTILE_HOSTNAMES") != "huantu.xyz,canvas.xinghengcode.shop":
        raise ValueError("Unexpected Turnstile domains")
    original_env = ENV_FILE.read_text()
    values = dict(line.split("=", 1) for line in original_env.splitlines() if "=" in line and not line.lstrip().startswith("#"))
    proxy_token = values.get("REVERSE_PROXY_TOKEN", "") or secrets.token_urlsafe(48)
    if not re.fullmatch(r"[A-Za-z0-9_-]{32,}", proxy_token):
        raise ValueError("Existing proxy token requires explicit correction")
    settings["REVERSE_PROXY_TOKEN"] = proxy_token

    request = urllib.request.Request("https://api.cloudflare.com/client/v4/ips", headers={"User-Agent": "infinite-canvas-security-setup"})
    with urllib.request.urlopen(request, timeout=20) as response:
        published = json.load(response)
    if not published.get("success"):
        raise ValueError("Cloudflare network discovery failed")
    ranges = []
    for version in (4, 6):
        networks = [ipaddress.ip_network(line) for line in published["result"]["ipv%d_cidrs" % version]]
        if not networks or any(net.version != version or net.prefixlen == 0 for net in networks):
            raise ValueError("Invalid Cloudflare network list")
        ranges.extend(str(net) for net in networks)
    snippet = "# Managed by configure-security.py; contains a private proxy token.\n"
    snippet += "".join("set_real_ip_from %s;\n" % network for network in ranges)
    snippet += 'real_ip_header CF-Connecting-IP;\nreal_ip_recursive on;\nproxy_set_header X-Real-IP $remote_addr;\nproxy_set_header X-Forwarded-For $remote_addr;\nproxy_set_header X-Forwarded-Proto $scheme;\n'
    snippet += 'proxy_set_header X-Proxy-Token "%s";\n' % proxy_token

    def update_block(block):
        changed = 0
        for node in block:
            if "block" in node:
                changed += update_block(node["block"])
        if any(node["directive"] == "proxy_pass" and node["args"] == ["http://127.0.0.1:3050"] for node in block):
            block[:] = [node for node in block if not (
                (node["directive"] == "proxy_set_header" and node["args"][0].lower() in ("x-real-ip", "x-forwarded-for", "x-forwarded-proto", "x-proxy-token"))
                or (node["directive"] == "include" and node["args"] == [str(SNIPPET)])
            )]
            block.append({"directive": "include", "args": [str(SNIPPET)]})
            changed += 1
        return changed

    planned = {SNIPPET: (snippet.encode(), 0o600)}
    for path in PROXY_FILES:
        parsed = crossplane.parse(str(path), single=True, comments=True, check_ctx=False)
        if parsed["status"] != "ok":
            raise ValueError("Cannot parse ingress file: " + str(path))
        directives = parsed["config"][0]["parsed"]
        if not update_block(directives):
            raise ValueError("Expected application upstream absent: " + str(path))
        planned[path] = ((crossplane.build(directives) + "\n").encode(), path.stat().st_mode & 0o777)
    remaining = [line for line in original_env.splitlines() if line.partition("=")[0].strip() not in settings]
    env_text = "\n".join(remaining) + "\n" + "".join(key + "=" + value + "\n" for key, value in settings.items())
    planned[ENV_FILE] = (env_text.encode(), 0o600)
    originals = {path: (path.read_bytes(), path.stat().st_mode & 0o777) if path.exists() else None for path in planned}
    try:
        for path, (data, mode) in planned.items():
            atomic_write(path, data, mode)
        subprocess.run([NGINX, "-t"], capture_output=True, check=True)
        subprocess.run([NGINX, "-s", "reload"], capture_output=True, check=True)
    except Exception:
        for path, original in originals.items():
            if original is None:
                path.unlink(missing_ok=True)
            else:
                atomic_write(path, *original)
        subprocess.run([NGINX, "-s", "reload"], capture_output=True)
        raise
    print(json.dumps({"result": "SECURITY_CONFIG_OK", "proxy_files": [str(path) for path in PROXY_FILES], "cloudflare_networks": len(ranges), "secret_values": "not logged"}))


def verify():
    result = subprocess.run(["docker", "logs", "--since", "30m", "infinite-canvas"], capture_output=True, text=True, check=True)
    probes = []
    for line in (result.stdout + result.stderr).splitlines():
        try:
            event = json.loads(line)
        except (ValueError, TypeError):
            continue
        if str(event.get("request_id", "")).startswith("security-ip-probe-"):
            address = ipaddress.ip_address(event["client_ip"])
            probes.append({"request_id": event["request_id"], "status": event["status"], "public_client_ip": address.is_global, "spoofed_ip_accepted": str(address) == "198.51.100.75"})
    print(json.dumps({"proxy_probes": probes}))
    if not probes or any(not p["public_client_ip"] or p["spoofed_ip_accepted"] for p in probes):
        raise SystemExit("Proxy verification failed or missing probe requests")


if __name__ == "__main__":
    try:
        if sys.argv[1] == "inspect":
            inspect()
        elif sys.argv[1] == "configure":
            configure(sys.argv[2])
        elif sys.argv[1] == "verify":
            verify()
        else:
            raise ValueError("Unknown operation")
    except Exception as error:
        print("Security configuration failed: " + type(error).__name__)
        raise SystemExit(1)
