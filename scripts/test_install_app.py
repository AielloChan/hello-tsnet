import importlib.util
import pathlib
import plistlib
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("install_app", pathlib.Path(__file__).with_name("install-app.py"))
installer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(installer)


class InstallTests(unittest.TestCase):
    def bundle(self, path, content):
        executable = path / "Contents/MacOS/hello-tsnet"
        executable.parent.mkdir(parents=True)
        executable.write_text(content)
        executable.chmod(0o755)
        with (path / "Contents/Info.plist").open("wb") as file:
            plistlib.dump({"CFBundleIdentifier": "org.hellotsnet.app", "CFBundleExecutable": "hello-tsnet"}, file)

    def test_update_stays_at_same_path_and_opens_exactly_once(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            source, destination = root / "build.app", root / "Applications/Hello TSNet.app"
            self.bundle(source, "new")
            self.bundle(destination, "old")
            (destination / "Contents/obsolete").write_text("old resource")
            with patch.object(installer, "stop_installed"), patch.object(installer, "run") as run:
                installer.install(source, destination, root / "previous.tar", launch=True)
            self.assertEqual((destination / "Contents/MacOS/hello-tsnet").read_text(), "new")
            self.assertFalse((destination / "Contents/obsolete").exists())
            launches = [call.args[0] for call in run.call_args_list if call.args[0][0] == "/usr/bin/open"]
            self.assertEqual(launches, [["/usr/bin/open", "-a", str(destination)]])

    def test_failed_verification_restores_old_app_and_does_not_launch(self):
        with tempfile.TemporaryDirectory() as temp:
            root = pathlib.Path(temp)
            source, destination = root / "build.app", root / "Applications/Hello TSNet.app"
            self.bundle(source, "new")
            self.bundle(destination, "old")
            calls = []
            def run(command):
                calls.append(command)
                if command[-1] == str(destination):
                    raise RuntimeError("verification failed")
            with patch.object(installer, "stop_installed"), patch.object(installer, "run", side_effect=run):
                with self.assertRaises(RuntimeError):
                    installer.install(source, destination, root / "previous.tar", launch=True)
            self.assertEqual((destination / "Contents/MacOS/hello-tsnet").read_text(), "old")
            self.assertEqual((destination / "Contents/MacOS/hello-tsnet").stat().st_mode & 0o777, 0o755)
            self.assertFalse(any(command[0] == "/usr/bin/open" for command in calls))


if __name__ == "__main__":
    unittest.main()
