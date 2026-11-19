import contextlib
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('boundary', Path(__file__).with_name('check-public-boundary.py'))
boundary = importlib.util.module_from_spec(spec)
spec.loader.exec_module(boundary)

class PublicationBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name).resolve()
        boundary.ROOT = self.root
        boundary.MANIFEST = self.root / 'public-files.json'
        self.good = self.root / 'public.ts'
        self.good.write_text('export const title = "Astradia";\n')
        self.approve()

    def tearDown(self):
        self.temp.cleanup()

    def approve(self):
        files = {p.relative_to(self.root).as_posix(): {'sha256': hashlib.sha256(p.read_bytes()).hexdigest()}
                 for p in self.root.rglob('*') if p.is_file() and p.name != 'public-files.json'}
        boundary.MANIFEST.write_text(json.dumps({'files': files}))

    def check(self):
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return boundary.main()

    def test_exact_reviewed_snapshot_passes(self):
        self.assertEqual(self.check(), 0)

    def test_unreviewed_file_and_modified_source_fail(self):
        (self.root / 'extra.ts').write_text('export const value = 1;')
        self.assertEqual(self.check(), 1)
        (self.root / 'extra.ts').unlink()
        self.good.write_text('export const title = "changed";')
        self.assertEqual(self.check(), 1)

    def test_private_paths_and_imports_fail_even_if_added_to_manifest(self):
        (self.root / 'engine.rs').write_text('// prohibited path')
        self.approve()
        self.assertEqual(self.check(), 1)
        (self.root / 'engine.rs').unlink()
        self.good.write_text("import { score } from '../server/lib/private';")
        self.approve()
        self.assertEqual(self.check(), 1)

    def test_private_module_references_fail_even_if_added_to_manifest(self):
        for name, text in [
            ('worker.rs', 'use crate::engine::evaluate;\n'),
            ('routes.rs', 'let plan = astradia::strategy::select();\n'),
        ]:
            (self.root / name).write_text(text)
            self.approve()
            self.assertEqual(self.check(), 1, name)
            (self.root / name).unlink()
        (self.root / 'signing.rs').write_text('use crate::http::http_client;\n')
        self.approve()
        self.assertEqual(self.check(), 0)

    def test_symlink_cannot_pull_in_external_files(self):
        (self.root / 'escape.ts').symlink_to(Path(__file__).resolve())
        self.assertEqual(self.check(), 1)

if __name__ == '__main__':
    unittest.main()
