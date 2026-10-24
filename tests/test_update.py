import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("certree_update", Path(__file__).resolve().parents[1] / "scripts" / "update.py")
update = importlib.util.module_from_spec(spec)
spec.loader.exec_module(update)


class UpdateTests(unittest.TestCase):
    def test_capture_preserves_existing_identity_key_trust_and_host(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            original = root / "testkit" / "compose.yaml"
            original.parent.mkdir()
            original.write_text("services: {}")
            labels = {"com.docker.compose.project": "existing-test", "com.docker.compose.service": "certree", "com.docker.compose.project.config_files": str(original)}
            app = {"Config": {"Labels": labels, "Env": ["CERTREE_MASTER_KEY=existing-key", "CERTREE_ADMIN_PASSWORD=p$a'ss", "CERTREE_TRUST_BUNDLE=/run/test-ca.pem"]}, "Mounts": [{"Type": "volume", "Name": "existing-test_certree_data", "Source": "/var/lib/docker/volumes/existing-test_certree_data/_data", "Destination": "/data"}]}
            proxy = {"Config": {"Env": ["TEST_HOST_IP=152.70.80.107"]}}
            config = {"services": {"certree": {"build": {"context": "/original/source", "dockerfile": "Dockerfile"}, "environment": {"CERTREE_MASTER_KEY": "existing-key", "CERTREE_ADMIN_PASSWORD": "p$a'ss"}, "volumes": [{"type": "volume", "source": "certree_data", "target": "/data"}, {"type": "bind", "source": "/original/ca.pem", "target": "/run/test-ca.pem"}]}, "proxy": {"ports": [{"published": "8443", "target": 443}]}}, "volumes": {"certree_data": {"name": "existing-test_certree_data"}}}
            def fake_run(args, **kwargs):
                if args[:3] == ["docker", "ps", "-aq"]:
                    return "app\nproxy\n"
                if args[:2] == ["docker", "inspect"]:
                    return json.dumps([app] if args[-1] == "existing-app" else [app, proxy])
                if args[-3:] == ["config", "--format", "json"]:
                    self.assertEqual(kwargs["env"]["TEST_HOST_IP"], "152.70.80.107")
                    self.assertEqual(kwargs["env"]["CERTREE_MASTER_KEY"], "existing-key")
                    return json.dumps(config)
                if args[:3] == ["docker", "volume", "inspect"]:
                    self.assertEqual(args[-1], "existing-test_certree_data")
                    return "[]"
                self.fail(str(args))
            with patch.object(update, "ROOT", root), patch.object(update, "MARKER", root / ".local" / "profile.json"), patch.object(update.Path, "home", return_value=root), patch.object(update, "run", side_effect=fake_run), patch.object(update.subprocess, "run") as execute:
                profile = update.capture("existing-app")
                saved = json.loads(Path(profile["compose"]).read_text())
                self.assertEqual(saved["volumes"]["certree_preserved_data"], {"external": True, "name": "existing-test_certree_data"})
                self.assertEqual(saved["services"]["certree"]["environment"]["CERTREE_MASTER_KEY"], "existing-key")
                self.assertEqual(saved["services"]["certree"]["environment"]["CERTREE_ADMIN_PASSWORD"], "p$$a'ss")
                self.assertEqual(saved["services"]["certree"]["volumes"][1]["source"], "/original/ca.pem")
                update.update(profile)
                commands = [call.args[0] for call in execute.call_args_list]
                self.assertEqual(commands[0][-2:], ["build", "certree"])
                self.assertEqual(commands[1][-5:], ["up", "-d", "--no-deps", "--wait", "certree"])
                self.assertFalse(any("down" in command or "proxy" in command or "-v" in command for command in commands))
                saved = json.loads(Path(profile["compose"]).read_text())
                self.assertEqual(saved["services"]["certree"]["build"]["context"], str(root))
                self.assertEqual(saved["services"]["certree"]["environment"]["CERTREE_ADMIN_PASSWORD"], "p$$a'ss")

    def test_missing_existing_volume_aborts_before_build(self):
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "compose.json"
            config.write_text(json.dumps({"services": {"certree": {"build": {}}}}))
            with patch.object(update, "run", side_effect=RuntimeError("volume missing")), patch.object(update.subprocess, "run") as execute:
                with self.assertRaisesRegex(RuntimeError, "volume missing"):
                    update.update({"compose": str(config), "project": "existing", "data_type": "volume", "data_source": "original"})
                execute.assert_not_called()

    def test_build_failure_does_not_replace_running_app(self):
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "compose.json"
            config.write_text(json.dumps({"services": {"certree": {"build": {}}}}))
            with patch.object(update, "run", return_value="[]"), patch.object(update.subprocess, "run", side_effect=update.subprocess.CalledProcessError(1, "docker build")) as execute:
                with self.assertRaises(update.subprocess.CalledProcessError):
                    update.update({"compose": str(config), "project": "existing", "data_type": "volume", "data_source": "original"})
                self.assertEqual(execute.call_count, 1)
                self.assertEqual(execute.call_args.args[0][-2:], ["build", "certree"])

    def test_git_pull_stops_for_modified_tracked_source(self):
        with tempfile.TemporaryDirectory() as directory:
            config = Path(directory) / "compose.json"
            config.write_text(json.dumps({"services": {"certree": {"build": {}}}}))
            def fake_run(args):
                return " M README.md" if args[:2] == ["git", "status"] else "[]"
            with patch.object(update, "run", side_effect=fake_run), patch.object(update.subprocess, "run") as execute:
                with self.assertRaisesRegex(RuntimeError, "Tracked source files"):
                    update.update({"compose": str(config), "project": "existing", "data_type": "volume", "data_source": "original"}, pull=True)
                execute.assert_not_called()

    def test_conflicting_keys_are_not_silently_replaced(self):
        with self.assertRaisesRegex(RuntimeError, "Conflicting"):
            update.environment([{"Config": {"Env": ["CERTREE_MASTER_KEY=a"]}}, {"Config": {"Env": ["CERTREE_MASTER_KEY=b"]}}])

    def test_missing_key_is_not_regenerated(self):
        with self.assertRaisesRegex(RuntimeError, "Existing encryption key missing"):
            update.environment([{"Config": {"Env": ["TEST_HOST_IP=localhost"]}}])


if __name__ == "__main__":
    unittest.main()
