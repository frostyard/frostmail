"""Exercise Lodge's mail app, the NSGo client, inside an authenticated, disposable Lodge.

Supply a tools directory containing lodge/{lodge-e2e,lodgectl,shell},
geckodriver and firefox/firefox. Run from the FrostMail repository root.

With --deb DEB it runs on what the .deb installs (make deb): dpkg-deb
extracts the package into the Lodge root, and the package's own maild and
web build run. Without it, it runs on make nsgo-lodge-app's staged app
directory (build/nsgo-lodge/apps/mail) and a maild built from this tree.
Firefox runs headless.
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


def smoke(tools, root, cleanup, deb=None):
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
    if deb is not None:
        subprocess.run(["dpkg-deb", "-x", str(deb), str(root)], check=True)
        maild = root/"usr/bin/maild"
        print(f"maild and the web build from {deb}")
    else:
        shutil.copytree("build/nsgo-lodge/apps/mail", root/"usr/share/lodge/apps/mail")
        subprocess.run(["go", "build", "-o", str(root)+"/", "./cmd/maild"], check=True)
        maild = root/"maild"
    if not (root/"usr/share/lodge/apps/mail/web/nsgo.json").is_file():
        raise RuntimeError("the mail app has no nsgo.json: it is not the NSGo client's web build")
    shutil.copytree(tools/"lodge/shell", root/"usr/share/lodge/shell")
    subprocess.run(["go", "build", "-o", str(root)+"/", "./tools/uifixture"], check=True)
    run([root/"uifixture", "-out", root/"data", "-showcase", "tools/uifixture/showcase"])
    origin = f"http://127.0.0.1:{port()}"
    start([tools/"lodge/lodge-e2e", "--root", root, "--listen", origin.removeprefix("http://"),
           "--content-listen", f"127.0.0.1:{port()}"])
    wait("Lodge control socket", (root/"run/lodge/control.sock").exists)
    for kind, subject in [("login", "e2e@example.com"), ("node", "e2e-node")]:
        run([tools/"lodge/lodgectl", "allow", kind, subject])
    start([maild, "--lodge"])
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
    def click(label):
        selector = json.dumps(f'[aria-label="{label}"]')
        wait(label, lambda: script(
            f"const el=document.querySelector({selector}); if(!el)return false; el.click(); return true"))

    click("Compose")
    field = wait("draft subject", lambda: script(
        """const root=document.querySelector('[aria-label="Subject"]');
        return root && (root.matches('input,textarea') ? root : root.querySelector('input,textarea'));"""))
    draft_title = "Edited in the Lodge browser"
    element_id = field["element-6066-11e4-a52e-4f735466cecf"]
    post(f"/element/{element_id}/value", {"text": draft_title})
    post("/frame", {"id": None})
    draft_url = wait("draft route", lambda: (
        url if "/compose/" in (url := request(browser+"/url")) else None))
    enter_mail()
    click("Save and close")
    post("/frame", {"id": None})
    wait("draft closed", lambda: request(browser+"/url") != draft_url)
    post("/url", {"url": draft_url})
    enter_mail()
    wait("saved subject at draft permalink", lambda: script(
        """const root=document.querySelector('[aria-label="Subject"]');
        const el=root && (root.matches('input,textarea') ? root : root.querySelector('input,textarea'));
        return el && el.value === 'Edited in the Lodge browser'"""))
    print("PASS: inbox, isolated reader, settings, Back, refresh and editable draft permalink")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tools", type=Path)
    parser.add_argument("--deb", type=Path, help="run on what this .deb installs (make deb's dist/frostyard-maild_*.deb)")
    args = parser.parse_args()
    with tempfile.TemporaryDirectory(prefix="fm-lodge-") as tmp, ExitStack() as cleanup:
        smoke(args.tools.resolve(), Path(tmp), cleanup, args.deb.resolve() if args.deb else None)
