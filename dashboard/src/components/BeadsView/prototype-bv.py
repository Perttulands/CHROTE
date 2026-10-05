#!/usr/bin/env python3
"""Disposable direct-bv trial. Source stores are read; bv writes only scratch state."""

import argparse
import concurrent.futures
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import urllib.request


def clean_env():
    env = os.environ.copy()
    for name in ("BEADS_DIR", "BEADS_DB", "BD_DB", "TMUX", "TMUX_PANE", "TMUX_TMPDIR"):
        env.pop(name, None)
    env["TERM"] = "xterm-256color"
    return env


def run(command, cwd=None, timeout=35):
    process = subprocess.Popen(command, cwd=cwd, env=clean_env(), start_new_session=True,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        out, err = process.communicate(timeout=timeout)
        if process.returncode:
            raise RuntimeError((err or out).strip()[-1500:])
        return out
    finally:
        # Only our own bd/tmux subprocess group, never an existing terminal session.
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass


def atomic(path, text):
    path.parent.mkdir(parents=True, exist_ok=True)
    pending = path.with_name(path.name + ".new")
    pending.write_text(text)
    pending.replace(path)


def load(state):
    return json.loads((state / "status.json").read_text())


def save(state, data):
    atomic(state / "status.json", json.dumps(data, indent=2) + "\n")


def fingerprint(project):
    store = Path(project) / ".beads"
    manifests = sorted((store / "embeddeddolt").glob("*/.dolt/noms/manifest"))
    if manifests:
        digest = hashlib.sha256()
        for path in manifests:
            digest.update(str(path).encode())
            digest.update(path.read_bytes())
        return digest.hexdigest()
    for name in ("beads.db", "issues.db", "issues.jsonl", "beads.jsonl"):
        path = store / name
        if path.exists():
            stat = path.stat()
            return f"{name}:{stat.st_mtime_ns}:{stat.st_size}"
    raise RuntimeError(f"No readable source fingerprint in {store}")


def read_store(entry, state, bd):
    result = dict(entry, attempted_at=time.time(), loading=False)
    started = time.monotonic()
    try:
        project = Path(entry["path"])
        expected = (project / ".beads").resolve()
        if not expected.is_dir():
            raise RuntimeError("Workspace does not own a .beads directory")
        if not entry.get("prefix"):
            identity = json.loads(run([bd, "--readonly", "where", "--json"], cwd=project))
            if Path(identity["path"]).resolve() != expected:
                raise RuntimeError(f"bd resolved {identity['path']}, not this workspace's {expected}")
            if not identity.get("prefix"):
                raise RuntimeError("bd where reports this store without an issue prefix; authoritative export identity unavailable")
            result["prefix"] = identity["prefix"]
            result["store"] = str(expected)
        target = state / "stores" / entry["key"] / ".beads" / "issues.jsonl"
        target.parent.mkdir(parents=True, exist_ok=True)
        temporary = target.with_name("export.pending")
        try:
            generation = fingerprint(project)
        except PermissionError:
            # The installed wrapper repairs temporary Dolt ACL rewrites before its read.
            # An unknown pre-read generation forces another read on the next check.
            generation = ""
        run([bd, "--readonly", "export", "--output", str(temporary)], cwd=project)
        records = [json.loads(line) for line in temporary.read_text().splitlines() if line.strip()]
        if not all(record.get("id") for record in records):
            raise RuntimeError("Export has records without source ids")
        temporary.replace(target)
        result.update(read_at=time.time(), count=len(records), error=None, error_origin=None,
                      fingerprint=generation, snapshot=str(target),
                      newest_update=max((r.get("updated_at", "") for r in records), default=""))
    except Exception as error:
        result["error"] = str(error)
        result["error_origin"] = "read"
    result["read_seconds"] = round(time.monotonic() - started, 3)
    return result


def workspace(state, data):
    # Native prefixes preserve IDs; ambiguous duplicate store prefixes get a path-key namespace.
    entries = [e for e in data["stores"] if e.get("snapshot")]
    prefixes = [e["prefix"] for e in entries]
    rows = ["name: CHROTE direct-bv prototype", "repos:"]
    for entry in entries:
        prefix = entry["prefix"] + "-"
        if prefixes.count(entry["prefix"]) > 1:
            prefix = entry["prefix"] + "-" + entry["key"][:6] + "-"
        entry["viewer_prefix"] = prefix
        rows += [f"  - name: {json.dumps(entry['path'])}",
                 f"    path: {json.dumps(str(state / 'stores' / entry['key']))}",
                 f"    prefix: {json.dumps(prefix)}"]
    rows += ["discovery:", "  enabled: false", "defaults:", "  beads_path: .beads"]
    atomic(state / ".bv" / "workspace.yaml", "\n".join(rows) + "\n")


def refresh(state, force=False):
    with (state / ".refresh.lock").open("a") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        refresh_locked(state, force)


def refresh_locked(state, force=False):
    data = load(state)
    selected = []
    status_changed = False
    for index, entry in enumerate(data["stores"]):
        try:
            changed = fingerprint(entry["path"]) != entry.get("fingerprint")
            if entry.get("error_origin") == "fingerprint":
                entry["error"] = None
                entry["error_origin"] = None
                status_changed = True
        except Exception as error:
            changed = True
            if entry.get("error_origin") != "read":
                entry["error"] = str(error)
                entry["error_origin"] = "fingerprint"
                status_changed = True
        retry = time.time() - entry.get("attempted_at", 0) >= 60
        needs_read = changed or entry.get("error_origin") == "read"
        if force or (needs_read and (not entry.get("error") or retry)):
            entry["loading"] = True
            selected.append((index, dict(entry)))
    if not selected:
        if status_changed:
            save(state, data)
        return
    save(state, data)
    started = time.monotonic()
    with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
        futures = {pool.submit(read_store, entry, state, data["bd"]): index
                   for index, entry in selected}
        for future in concurrent.futures.as_completed(futures):
            data["stores"][futures[future]] = future.result()
            workspace(state, data)
            save(state, data)
    data["last_refresh_seconds"] = round(time.monotonic() - started, 3)
    data["last_refresh_at"] = time.time()
    save(state, data)


def status(state, scope, full=False):
    data = load(state)
    entries = data["stores"]
    if scope == "project":
        entries = [e for e in entries if e["path"] == data["project"]]
    ok = sum(bool(e.get("snapshot")) for e in entries)
    failed = sum(bool(e.get("error")) for e in entries)
    loading = sum(bool(e.get("loading")) for e in entries)
    if scope == "host":
        loaded_path = state / "host-loaded-at.txt"
        loaded = float(loaded_path.read_text()) if loaded_path.exists() else None
        age = f"loaded {int(time.time()-loaded)}s ago" if loaded else "not opened"
        message = f"PROTOTYPE {ok}/{len(entries)} sources | {failed} failed | {age} | q, Enter reload"
    else:
        read = entries[0].get("read_at") if entries else None
        age = f"read {int(time.time()-read)}s ago" if read else "no snapshot"
        message = f"PROTOTYPE {age} | {'refreshing' if loading else 'change-driven feed'} | {failed} failed"
    if not full:
        print(message)
        return
    print(message)
    print("Native host workspace is frozen until q then Enter; project JSONL live-reloads.")
    print("Source fingerprints checked every 2s; changed stores exported at concurrency 2.")
    print("New stores: relaunch discovery. Failed reads retried after 60s.")
    print("Snapshots are read-only product trials: viewer edits do not write back to live stores.\n")
    for entry in entries:
        read = time.strftime("%H:%M:%S UTC", time.gmtime(entry["read_at"])) if entry.get("read_at") else "never"
        print(f"{entry['path']}/.beads | {entry.get('prefix', '?')} | {entry.get('count', '?')} issues | {read} | {entry.get('read_seconds', '?')}s")
        if entry.get("error"):
            print("  UNAVAILABLE / previous snapshot retained: " + entry["error"])
        elif entry.get("loading"):
            print("  Refreshing changed source; previous snapshot retained")


def watch(state):
    while True:
        refresh(state)
        print("\033[2J\033[H", end="")
        status(state, "host", full=True)
        sys.stdout.flush()
        time.sleep(2)


def view(state, scope):
    data = load(state)
    # /tmp is excluded from CHROTE discovery; no scratch .beads becomes a work store.
    os.chdir(data["scratch_cwd"])
    while True:
        data = load(state)
        if scope == "host":
            atomic(state / "host-loaded-at.txt", str(time.time()))
            args = [data["bv"], "--workspace", str(state / ".bv" / "workspace.yaml")]
        else:
            entry = next(e for e in data["stores"] if e["path"] == data["project"])
            args = [data["bv"], "--db", str(Path(entry["snapshot"]).parent)]
        subprocess.run(args, env=clean_env())
        print("\nPROTOTYPE: Enter reopens the latest feed; type q then Enter to stop this viewer.")
        if input().strip().lower() == "q":
            return


def launch(args):
    state = args.state.resolve()
    state.mkdir(parents=True, exist_ok=True)
    tmux = str(args.tmux.resolve())
    command = [tmux, "-S", str(args.socket.resolve())]
    names = ["prototype-bv-host", "prototype-bv-project"]
    existing = run(command + ["list-sessions", "-F", "#{session_name}"]).splitlines()
    if any(name in existing for name in names):
        raise RuntimeError("Prototype sessions already exist; use refresh or the existing sessions")
    inventory = run(command + ["list-panes", "-a", "-F",
                               "#{session_name} #{pane_id} #{pane_pid} #{pane_width}x#{pane_height} #{window-size}"])
    atomic(state / "panes-before.txt", inventory)
    with urllib.request.urlopen(args.api_url.rstrip("/") + "/api/workspaces", timeout=20) as response:
        discovered = json.load(response)
    stores = [{"path": e["path"], "key": hashlib.sha256(e["path"].encode()).hexdigest()[:12]}
              for e in discovered if "store" in e["sources"]]
    project = str(args.project.resolve())
    if not any(e["path"] == project for e in stores):
        raise RuntimeError("Selected project is not an owning store in CHROTE discovery")
    save(state, {"project": project, "stores": stores, "bd": shutil.which("bd"),
                 "bv": shutil.which("bv"), "scratch_cwd": tempfile.mkdtemp(prefix="chrote-bv-prototype-")})
    refresh(state, force=True)
    data = load(state)
    if not any(e.get("snapshot") for e in data["stores"] if e["path"] == project):
        raise RuntimeError("Selected project source unavailable; inspect status.json")
    script = str(Path(__file__).resolve())
    for name, scope in zip(names, ("host", "project")):
        viewer = shlex.join([sys.executable, script, "view", "--state", str(state), "--scope", scope])
        pane = run(command + ["new-session", "-d", "-P", "-F", "#{pane_id}",
                              "-s", name, "-n", "viewer", "-c", data["scratch_cwd"], viewer]).strip()
        size = subprocess.run(command + ["-C", "attach-session", "-t", "=" + name],
                              input="refresh-client -C 160,48\n", text=True, env=clean_env(),
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
        if size.returncode:
            raise RuntimeError(size.stderr)
        check = run(command + ["display-message", "-p", "-t", pane,
                               "#{window_width}x#{window_height} #{window-size}"]).strip()
        if check != "160x48 latest":
            raise RuntimeError(f"New prototype size is {check}, expected 160x48 latest")
        bar = shlex.join([sys.executable, script, "status", "--state", str(state), "--scope", scope])
        run(command + ["set-option", "-t", pane, "status-right", "#(" + bar + ")"])
        run(command + ["set-option", "-t", pane, "status-right-length", "120"])
        source_cmd = shlex.join([sys.executable, script, "watch" if scope == "host" else "sources",
                                "--state", str(state), "--scope", scope])
        run(command + ["new-window", "-d", "-t", name + ":", "-n", "sources", "-c",
                       data["scratch_cwd"], source_cmd])
        print(f"{name}: {pane}, viewer window 0; sources window 1; {check}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("launch", "refresh", "watch", "view", "status", "sources"))
    parser.add_argument("--state", type=Path, required=True)
    parser.add_argument("--scope", choices=("host", "project"), default="host")
    parser.add_argument("--api-url")
    parser.add_argument("--project", type=Path, default=Path.cwd())
    parser.add_argument("--socket", type=Path)
    parser.add_argument("--tmux", type=Path, default=Path(shutil.which("tmux") or "tmux"))
    args = parser.parse_args()
    if args.action == "launch":
        if not args.api_url or not args.socket:
            parser.error("launch needs --api-url and --socket")
        launch(args)
    elif args.action == "refresh":
        refresh(args.state.resolve(), force=True)
    elif args.action == "watch":
        watch(args.state.resolve())
    elif args.action == "view":
        view(args.state.resolve(), args.scope)
    elif args.action == "status":
        status(args.state.resolve(), args.scope)
    else:
        while True:
            print("\033[2J\033[H", end="")
            status(args.state.resolve(), args.scope, full=True)
            sys.stdout.flush()
            time.sleep(2)


if __name__ == "__main__":
    main()
