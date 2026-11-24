"""Exercise the staged NSGo client inside an authenticated, disposable Lodge.

Supply a tools directory containing lodge/{lodge-e2e,lodgectl,shell},
geckodriver and firefox/firefox. Run from the FrostMail repository root.
"""
import argparse
from contextlib import ExitStack
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import tempfile
import time
from urllib.error import URLError
from urllib.request import Request, urlopen


def port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def wait(description, predicate):
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            result = predicate()
            if result:
                return result
        except (URLError, ConnectionError):
            pass
        time.sleep(0.15)
    raise RuntimeError(f"Timed out waiting for {description}")


def request(url, method="GET", data=None):
    body = json.dumps(data).encode() if data is not None else None
    req = Request(url, method=method, data=body, headers={"Content-Type": "application/json"})
    with urlopen(req, timeout=25) as response:
        return json.load(response)["value"]


def smoke(tools, root, cleanup):
    env = dict(os.environ, LODGE_ROOT=str(root), FROSTMAIL_DATA_DIR=str(root/"data"),
               FROSTMAIL_CACHE_DIR=str(root/"cache"), XDG_CONFIG_HOME=str(root/"config"),
               XDG_STATE_HOME=str(root/"state"), FROSTMAIL_SECRETS="file",
               FROSTMAIL_SOCKET=str(root/"maild.sock"), FROSTMAIL_SYNC="off")

    def start(args):
        child = subprocess.Popen(args, env=env)
        def stop():
            child.terminate()
            try:
                child.wait(timeout=5)
            except subprocess.TimeoutExpired:
                child.kill()
                child.wait()
        cleanup.callback(stop)

    def run(args):
        subprocess.run(args, env=env, check=True)

    (root/"run/lodge/apps").mkdir(parents=True)
    shutil.copytree(tools/"lodge/shell", root/"usr/share/lodge/shell")
    shutil.copytree("build/nsgo-lodge/apps/mail", root/"usr/share/lodge/apps/mail")
    run(["go", "build", "-o", str(root)+"/", "./cmd/maild", "./tools/uifixture"])
    run([root/"uifixture", "-out", root/"data", "-showcase", "tools/uifixture/showcase"])
    origin = f"http://127.0.0.1:{port()}"
    start([tools/"lodge/lodge-e2e", "--root", root, "--listen", origin.removeprefix("http://"),
           "--content-listen", f"127.0.0.1:{port()}"])
    wait("Lodge control socket", (root/"run/lodge/control.sock").exists)
    for kind, subject in [("login", "e2e@example.com"), ("node", "e2e-node")]:
        run([tools/"lodge/lodgectl", "allow", kind, subject])
    start([root/"maild", "--lodge"])
    wait("maild app socket", (root/"run/lodge/apps/mail.sock").exists)
    run([tools/"lodge/lodgectl", "reload"])

    driver_port = port()
    driver = f"http://127.0.0.1:{driver_port}"
    start([tools/"geckodriver", "--port", str(driver_port)])
    wait("WebDriver", lambda: request(driver+"/status")["ready"])
    config = {"browserName": "firefox", "moz:firefoxOptions": {
        "binary": str(tools/"firefox/firefox"), "args": ["-headless"]}}
    session = request(driver+"/session", "POST", {"capabilities": {"alwaysMatch": config}})
    browser = driver+"/session/"+session["sessionId"]
    cleanup.callback(request, browser, "DELETE")

    def post(path, data):
        return request(browser+path, "POST", data)

    def script(code):
        return post("/execute/sync", {"script": code, "args": []})

    def enter_mail():
        frame = wait("Mail frame", lambda: post("/element", {
            "using": "css selector", "value": 'iframe.frame[title="Mail"]'}))
        post("/frame", {"id": frame})

    post("/url", {"url": origin+"/mail"})
    enter_mail()
    subject = "Your trip to Lisbon: itinerary"
    wait("fixture inbox", lambda: script(f"return document.body.innerText.includes({json.dumps(subject)})"))
    script(f'document.querySelector(\'[aria-label="{subject}"]\').click()')
    wait("isolated content frame", lambda: script('return !!document.querySelector("iframe[src]")'))
    post("/frame", {"id": None})
    permalink = wait("selected message route", lambda: (
        url if "/message/" in (url := request(browser+"/url")) else None))
    enter_mail()
    script('document.querySelector(\'[aria-label="Settings"]\').click()')
    post("/frame", {"id": None})
    wait("settings URL", lambda: request(browser+"/url").endswith("/mail/settings"))
    enter_mail()
    wait("settings deep link", lambda: script('return !!document.querySelector(\'[aria-label="Add account"]\')'))
    post("/frame", {"id": None})
    post("/back", {})
    wait("original message URL", lambda: request(browser+"/url") == permalink)
    enter_mail()
    wait("restored reader", lambda: script('return !!document.querySelector("iframe[src]")'))
    post("/frame", {"id": None})
    post("/refresh", {})
    enter_mail()
    wait("direct message link", lambda: script('return !!document.querySelector("iframe[src]")'))
    print("PASS: fixture inbox, isolated message content, settings link and browser Back")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tools", type=Path)
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="fm-lodge-") as tmp, ExitStack() as cleanup:
        smoke(args.tools.resolve(), Path(tmp), cleanup)
