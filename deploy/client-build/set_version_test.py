import json
import sys
import tempfile
import unittest
from pathlib import Path

import set_version


class SetVersionTest(unittest.TestCase):
    def test_writes_package_tauri_and_cargo(self):
        with tempfile.TemporaryDirectory() as raw:
            root = Path(raw)
            desktop = root / "desktop" / "src-tauri"
            desktop.mkdir(parents=True)
            (root / "desktop" / "package.json").write_text(
                json.dumps({"name": "app", "version": "0.1.0"}) + "\n", encoding="utf-8"
            )
            (desktop / "tauri.conf.json").write_text(
                json.dumps({"productName": "FlammeBible", "version": "0.1.0"}) + "\n",
                encoding="utf-8",
            )
            (desktop / "Cargo.toml").write_text(
                '[package]\nname = "flammebible-desktop"\nversion = "0.1.0"\n\n'
                '[dependencies]\nserde = "1"\n',
                encoding="utf-8",
            )
            sys.argv = ["set_version.py", str(root), "1.2.0"]
            set_version.main()
            package = json.loads((root / "desktop" / "package.json").read_text(encoding="utf-8"))
            tauri = json.loads((desktop / "tauri.conf.json").read_text(encoding="utf-8"))
            cargo = (desktop / "Cargo.toml").read_text(encoding="utf-8")
            self.assertEqual(package["version"], "1.2.0")
            self.assertEqual(tauri["version"], "1.2.0")
            self.assertIn('version = "1.2.0"', cargo)
            self.assertIn('serde = "1"', cargo)


if __name__ == "__main__":
    unittest.main()
