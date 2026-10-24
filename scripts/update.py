#!/usr/bin/env python3
"""Update only the application, retaining an existing Compose installation."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parent.parent
MARKER = ROOT / ".local" / "update-profile.json"


def run(args, *, env=None):
    result = subprocess.run(args, cwd=ROOT, env=env, text=True, capture_output=True)
    if result.returncode:
        raise RuntimeError("Command failed: " + " ".join(args[:4]) + "\n" + result.stderr.strip())
    return result.stdout


def save_private(path, value):
    path.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    os.chmod(path.parent, 0o700)
    fd, temporary = tempfile.mkstemp(prefix=".certree-", dir=path.parent)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            json.dump(value, stream, indent=2)
            stream.write("\n")
        os.chmod(temporary, 0o600)
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def environment(containers):
    values = {}
    for container in containers:
        for item in container["Config"].get("Env", []):
            key, _, value = item.partition("=")
            if key.startswith(("CERTREE_", "TEST_")):
                if key in values and values[key] != value:
                    raise RuntimeError("Conflicting installation variable: " + key)
                values[key] = value
    if not values.get("CERTREE_MASTER_KEY"):
        raise RuntimeError("Existing encryption key missing; refusing to create an installation")
    return values


def literal_compose(value):
    # A resolved Compose config is parsed again on the next invocation. Preserve
    # literal dollar signs in passwords and all other resolved string values.
    if isinstance(value, str):
        return value.replace("$", "$$")
    if isinstance(value, list):
        return [literal_compose(item) for item in value]
    if isinstance(value, dict):
        return {key: literal_compose(item) for key, item in value.items()}
    return value


def capture(container, env_file=None):
    app = json.loads(run(["docker", "inspect", container]))[0]
    labels = app["Config"].get("Labels", {})
    project = labels.get("com.docker.compose.project", "")
    if labels.get("com.docker.compose.service") != "certree" or not re.fullmatch(r"[a-z0-9][a-z0-9_-]*", project):
        raise RuntimeError("Choose the existing Compose certree application container")
    paths = labels.get("com.docker.compose.project.config_files", "").split(",")
    if not paths or not all(Path(path).is_file() for path in paths):
        raise RuntimeError("Original Compose files are missing; restore them before capturing")
    ids = run(["docker", "ps", "-aq", "--filter", "label=com.docker.compose.project=" + project]).split()
    containers = json.loads(run(["docker", "inspect", *ids]))
    values = environment(containers)
    # Let Compose load the original .env if present; recovered runtime values win.
    execution_env = dict(os.environ, **values)
    command = ["docker", "compose", "-p", project]
    if env_file:
        command += ["--env-file", str(Path(env_file).resolve())]
    for path in paths:
        command += ["-f", path]
    config = json.loads(run(command + ["config", "--format", "json"], env=execution_env))
    if not isinstance(config.get("services", {}).get("certree", {}).get("build"), dict):
        raise RuntimeError("This installation does not build certree from source")
    config["services"]["certree"].setdefault("environment", {}).update(
        {key: value for key, value in values.items() if key.startswith("CERTREE_")}
    )
    mounts = [mount for mount in app.get("Mounts", []) if mount.get("Destination") == "/data"]
    if len(mounts) != 1:
        raise RuntimeError("Cannot identify the existing /data mount")
    mount = mounts[0]
    # Lock the captured DB mount to its current identity. Never create a new DB.
    if mount["Type"] == "volume":
        config.setdefault("volumes", {})["certree_preserved_data"] = {"external": True, "name": mount["Name"]}
        source = "certree_preserved_data"
    elif mount["Type"] == "bind":
        source = mount["Source"]
        if not Path(source).is_dir():
            raise RuntimeError("Existing data directory is unavailable")
    else:
        raise RuntimeError("Unsupported database mount type")
    data_mounts = config["services"]["certree"].get("volumes", [])
    for entry in data_mounts:
        if entry.get("target") == "/data":
            entry.update(type=mount["Type"], source=source)
            break
    else:
        raise RuntimeError("Original configuration has no /data mount")
    state = Path.home() / ".config" / "certree" / project / "compose.json"
    save_private(state, literal_compose(config))
    profile = {"project": project, "compose": str(state), "data_type": mount["Type"], "data_source": mount.get("Name", mount["Source"])}
    save_private(MARKER, profile)
    print("Saved existing installation profile for " + project + ". Secrets were not printed.")
    return profile


def update(profile, pull=False):
    config_path = Path(profile["compose"])
    config = json.loads(config_path.read_text(encoding="utf-8"))
    if profile["data_type"] == "volume":
        run(["docker", "volume", "inspect", profile["data_source"]])
    elif not Path(profile["data_source"]).is_dir():
        raise RuntimeError("Existing data directory is missing; refusing to start")
    if pull:
        run(["git", "rev-parse", "--is-inside-work-tree"])
        if run(["git", "status", "--porcelain", "--untracked-files=no"]).strip():
            raise RuntimeError("Tracked source files have local changes; commit or resolve them before pulling")
        subprocess.run(["git", "pull", "--ff-only"], cwd=ROOT, check=True)
    build = config["services"]["certree"]["build"]
    build["context"] = str(ROOT).replace("$", "$$")
    build["dockerfile"] = "Dockerfile"
    save_private(config_path, config)
    command = ["docker", "compose", "-p", profile["project"], "-f", str(config_path)]
    # Build first so a build failure cannot interrupt the currently running app.
    subprocess.run(command + ["build", "certree"], cwd=ROOT, check=True)
    subprocess.run(command + ["up", "-d", "--no-deps", "--wait", "certree"], cwd=ROOT, check=True)
    subprocess.run(command + ["ps", "certree"], cwd=ROOT, check=True)
    print("Application updated using the existing database, key, trust mounts and project. Proxy unchanged.")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--container", help="Capture the existing certree container before updating")
    parser.add_argument("--env-file", help="Optional original installation environment file for capture")
    parser.add_argument("--capture-only", action="store_true", help="Save configuration without rebuilding or restarting")
    parser.add_argument("--pull", action="store_true", help="Pull the configured Git branch with --ff-only before building")
    args = parser.parse_args()
    if args.env_file and not args.container:
        parser.error("--env-file requires --container")
    if args.container:
        profile = capture(args.container, args.env_file)
    elif MARKER.is_file():
        profile = json.loads(MARKER.read_text(encoding="utf-8"))
    else:
        parser.error("First run requires --container EXISTING_CERTREE_CONTAINER")
    if not args.capture_only:
        update(profile, args.pull)


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, OSError, ValueError, subprocess.CalledProcessError) as error:
        print("Update stopped: " + str(error), file=sys.stderr)
        sys.exit(1)
