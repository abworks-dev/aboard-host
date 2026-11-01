#!/usr/bin/env python3
"""Stdlib synthetic Git fixtures only; no business/runtime tests or source repair.

All fixture writes stay in this repository's ignored .data/logs subtree.
Retain fixtures for diagnosis; run with python -B to prevent bytecode artifacts.
"""
from contextlib import redirect_stdout
import hashlib
import io
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from types import SimpleNamespace
from unittest.mock import patch

sys.dont_write_bytecode = True
import check_docs as guard


class GuardTests(unittest.TestCase):
    def setUp(self):
        parent = guard.ROOT / '.data/logs/docs-maintenance-guard-20261029-host-owner/fixtures'
        parent.mkdir(parents=True, exist_ok=True)
        self.root = Path(tempfile.mkdtemp(prefix='case-', dir=parent))
        self.git('init', '-q')
        self.put('.gitignore', '.data/\ndelivery/\nignored.md\n')
        self.put('README.md', '# Example\n')
        self.commit()

    def git(self, *args):
        result = subprocess.run(['git', '-C', str(self.root), *args],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(result.returncode, 0, result.stderr.decode('utf-8', 'replace'))
        return result.stdout

    def put(self, name, text):
        path = self.root / name
        self.assertTrue(path.resolve().is_relative_to(self.root.resolve()))
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text, encoding='utf-8')

    def commit(self):
        self.git('add', '.')
        self.git('-c', 'user.name=Synthetic Fixture', '-c', 'user.email=fixture@example.invalid',
                 '-c', 'commit.gpgsign=false', 'commit', '-qm', 'synthetic baseline')

    def findings(self, **kwargs):
        return guard.audit(self.root, **kwargs)[2]

    def errors(self, **kwargs):
        return [f for f in self.findings(**kwargs) if f[0] == 'ERROR']

    def assert_error(self, kind, **kwargs):
        self.assertIn(kind, [f[2] for f in self.errors(**kwargs)])

    def test_clean_baseline(self):
        self.assertEqual(self.errors(), [])

    def test_invalid_base_never_falls_back(self):
        with self.assertRaises(ValueError):
            self.findings(base='definitely-not-a-ref')
        with self.assertRaises(ValueError):
            self.findings(base='--help')

    def test_new_bad_link_fails(self):
        self.put('README.md', '# Example\n[broken](docs/missing.md)\n')
        self.assert_error('missing-link')

    def test_history_missing_warns_but_new_missing_fails(self):
        self.put('README.md', '# Example\n[old](docs/old-missing.md)\n')
        self.commit()
        self.put('README.md', '# Example\n[old](docs/old-missing.md)\n[new](docs/new-missing.md)\n')
        findings = self.findings()
        self.assertEqual(sum(f[0] == 'WARN' for f in findings), 1)
        self.assertEqual(sum(f[0] == 'ERROR' for f in findings), 1)

    def test_same_destination_new_regression_not_baseline_debt(self):
        self.put('README.md', '# Example\n[x](docs/target.md#here)\n')
        self.put('docs/target.md', '# Here\n')
        self.commit()
        self.put('docs/target.md', '# Gone\n')
        self.assert_error('missing-fragment')

    def test_removing_target_with_unchanged_source_fails(self):
        self.put('README.md', '# Example\n[x](docs/target.md)\n')
        self.put('docs/target.md', '# Target\n')
        self.commit()
        target = self.root / 'docs/target.md'
        self.assertTrue(target.resolve().is_relative_to(self.root.resolve()))
        target.unlink()
        self.assert_error('missing-link')

    def test_directory_target_baseline_is_real_and_removal_fails(self):
        self.put('README.md', '# Example\n[x](docs/section/)\n')
        self.put('docs/section/note.md', '# Note\n')
        self.commit()
        self.assertEqual(self.errors(), [])
        target = self.root / 'docs/section'
        self.assertTrue(target.resolve().is_relative_to(self.root.resolve()))
        shutil.rmtree(target)
        self.assert_error('missing-link')

    def test_removed_unused_heading_still_compatibility_failure(self):
        self.put('README.md', '# Example\n## Stable\n')
        self.commit()
        self.put('README.md', '# Example\n## Replacement\n')
        self.assert_error('removed-anchor')

    def test_explicit_alias_keeps_old_heading(self):
        self.put('README.md', '# Example\n## Stable\n')
        self.commit()
        self.put('README.md', '# Example\n<a id="stable"></a>\n## Replacement\n')
        self.assertEqual(self.errors(), [])

    def test_duplicate_heading_new_fails_old_warns(self):
        self.put('README.md', '# Example\n## Repeat\n## Repeat\n')
        self.assert_error('duplicate-heading')
        self.commit()
        self.assertFalse(self.errors())
        self.assertIn('duplicate-heading', [f[2] for f in self.findings() if f[0] == 'WARN'])

    def test_duplicate_explicit_anchor(self):
        self.put('README.md', '# Example\n<a id="x"></a>\n<div id="x"></div>\n')
        self.assert_error('duplicate-anchor')

    def test_stable_definition_deletion_all_supported_classes(self):
        for name, content, token in [
            ('specs/a/spec.md', '### User Story 1 - story\n', 'US1'),
            ('specs/a/spec.md', '- **FR-001**: requirement\n', 'FR-001'),
            ('specs/a/spec.md', '- **SC-001**: criterion\n', 'SC-001'),
            ('specs/a/tasks.md', '- [ ] T001 prepare without US token\n', 'T001'),
            ('specs/a/tasks.md', '### V001 — verification\n', 'V001'),
            ('specs/a/tasks.md', '| 门 | 依据 |\n| --- | --- |\n| G306 | exact gate |\n', 'G306')]:
            with self.subTest(token=token):
                self.put(name, content)
                self.commit()
                self.put(name, '')
                self.assertTrue(any(f[2] == 'removed-id' and f[3] == token for f in self.errors()))
                self.commit()

    def test_duplicate_ids_and_mentions_are_not_definitions(self):
        self.put('specs/a/tasks.md', '- [ ] T001 first\n- [ ] T001 second\n')
        self.assert_error('duplicate-id')
        parsed = guard.parse('### User Story 1 - a\n\nUS1 FR-001 T001 V001 G306\n'
                             '| 导航 | target |\n| --- | --- |\n| V001 | reference |\n'
                             '| V001 | reference twice |\n', 'specs/a/spec.md')
        self.assertEqual(parsed.ids, {'US1': 1})

    def test_gate_definition_and_navigation_not_duplicate(self):
        text = ('| 门 | 依据 |\n| --- | --- |\n| G306 / T018 | body |\n\n'
                '| 导航 | 位置 |\n| --- | --- |\n| G306 | reference |\n'
                '| G306 | reference again |\n')
        self.assertEqual(guard.parse(text, 'specs/a/tasks.md').ids, {'G306': 1})

    def test_checkbox_entire_line_preserved_in_maintenance(self):
        self.put('specs/a/tasks.md', '- [ ] T001 [US1] body `source.go`\n')
        self.commit()
        self.put('specs/a/tasks.md', '- [x] T001 [US1] body `changed.go`\n')
        self.assert_error('changed-checkbox-line')
        self.assert_error('task-update-review-required', mode='development')
        self.assertEqual(self.errors(mode='development', reviewed_task_updates=True), [])

    def test_checkbox_removal_development_does_not_remove_id_protection(self):
        self.put('specs/a/tasks.md', '- [ ] T001 body\n')
        self.commit()
        self.put('specs/a/tasks.md', '')
        self.assert_error('removed-id', mode='development')

    def test_code_examples_do_not_create_links_ids_or_headings(self):
        text = ('# Example\n```md\n# Fake\n- **FR-009**: no\n[x](bad.md)\n```\n'
                '~~~\n- [ ] T009 nope\n~~~\n    [x](bad.md)\n'
                '`[x](bad.md)` and `FR-999`\n<!-- [x](bad.md) -->\n')
        self.put('README.md', text)
        self.assertEqual(self.errors(), [])
        self.assertEqual(guard.parse(text, 'README.md').ids, {})

    def test_space_chinese_percent_encoding_and_same_doc_fragment(self):
        self.put('docs/中文 文件.md', '# 标题 一\n<a id="explicit"></a>\n')
        self.put('README.md', '# Example\n[x](<docs/中文 文件.md#标题-一>)\n'
                 '[y](docs/%E4%B8%AD%E6%96%87%20%E6%96%87%E4%BB%B6.md#explicit)\n[x](#example)\n')
        self.assertEqual(self.errors(), [])

    def test_reference_images_and_definitions_are_checked(self):
        self.put('docs/a.md', '# Here\n')
        self.put('docs/a.png', 'synthetic-not-a-real-image')
        self.put('README.md', '# Example\n[x][ref]\n![picture][image]\n[ref]: docs/a.md#here\n'
                 '[image]: docs/a.png\n[short]\n[short]: docs/a.md\n')
        self.assertEqual(self.errors(), [])
        self.put('README.md', '# Example\n[x][ref]\n![picture](docs/missing.png)\n[ref]: docs/a.md#missing\n')
        self.assert_error('missing-fragment')
        self.assert_error('missing-link')

    def test_unresolved_full_reference_and_source_lines_are_unverified(self):
        self.put('docs/a.md', '# Here\n')
        self.put('README.md', '# Example\n[x][not-defined]\n[line](docs/a.md#L999999)\n')
        self.assertEqual(self.errors(), [])
        self.assertEqual(sum(f[0] == 'UNVERIFIED' for f in self.findings()), 2)

    def test_boundaries_not_accessed(self):
        self.put('README.md', '# Example\n[x](../../not-repository.md)\n'
                 '[x](C:/machine/file.md)\n[x](https://invalid.invalid/file.md)\n'
                 '[x](.data/history.md#missing)\n[x](delivery/frozen.md#missing)\n')
        findings = self.findings()
        self.assertFalse(self.errors())
        self.assertEqual(sum(f[0] == 'UNVERIFIED' for f in findings), 5)

    def test_ignored_and_frozen_markdown_not_discovered(self):
        for path in ['.data/broken.md', 'delivery/broken.md', 'docs/archive/broken.md',
                     'docs/experiments/broken.md']:
            self.put(path, '# Broken\n[x](not-existing.md)\n')
        self.assertEqual(guard.audit(self.root)[1], ['README.md'])

    def test_real_symlink_to_ignored_invalid_utf8_is_never_read(self):
        ignored = self.root / '.data/history.md'
        ignored.parent.mkdir(parents=True, exist_ok=True)
        ignored.write_bytes(b'\xff\xfe\x80-not-utf8')
        alias = self.root / 'docs/link.md'
        alias.parent.mkdir(parents=True, exist_ok=True)
        try:
            alias.symlink_to(ignored)
        except OSError as exc:
            self.skipTest('OS did not permit a real symlink: ' + str(exc))
        self.put('README.md', '# Example\n[x](docs/link.md#bad)\n')
        findings = self.findings()
        self.assertFalse(self.errors())
        self.assertTrue(any(f[0] == 'UNVERIFIED' and f[2] == 'source-boundary' for f in findings))
        self.assertTrue(any(f[0] == 'UNVERIFIED' and f[2] == 'boundary' for f in findings))

    def test_reparse_component_is_rejected_without_reading_target(self):
        self.put('docs/link/file.md', '# Visible\n')
        self.put('README.md', '# Example\n[x](docs/link/file.md#bad)\n')
        original = Path.lstat
        component = self.root / 'docs/link'
        def lstat(path, *args, **kwargs):
            if path == component:
                return SimpleNamespace(st_mode=0o040755, st_file_attributes=0x400)
            return original(path, *args, **kwargs)
        with patch.object(Path, 'lstat', lstat):
            with self.assertRaisesRegex(ValueError, 'symlink/reparse boundary'):
                guard.safe_path(self.root, 'docs/link/file.md')
            findings = self.findings()
        self.assertFalse([f for f in findings if f[0] == 'ERROR'])
        self.assertTrue(any(f[2] == 'source-boundary' for f in findings))
        self.assertTrue(any(f[0] == 'UNVERIFIED' and f[2] == 'boundary' for f in findings))

    def test_constitution_explicit_scope_not_other_generated_markdown(self):
        self.put('.specify/memory/constitution.md', '# Constitution\n[x](missing.md)\n')
        self.put('.specify/templates/example.md', '# Excluded\n[x](missing.md)\n')
        self.put('tools/examples.md', '# Excluded\n[x](missing.md)\n')
        scope = guard.audit(self.root)[1]
        self.assertIn('.specify/memory/constitution.md', scope)
        self.assertNotIn('.specify/templates/example.md', scope)
        self.assertNotIn('tools/examples.md', scope)
        self.assert_error('missing-link')

    def test_limited_files_do_not_claim_reverse_reference_scan(self):
        self.put('README.md', '# Example\n[x](docs/a.md#here)\n')
        self.put('docs/a.md', '# Here\n')
        self.commit()
        self.put('docs/a.md', '# Else\n<a id="here"></a>\n')
        self.put('README.md', '# Example\n[x](docs/no.md)\n')
        self.assertEqual(self.errors(files=['docs/a.md']), [])
        self.assert_error('missing-link')

    def test_reject_invalid_empty_or_external_files_scope(self):
        for files in [[], ['docs/not-existing.md'], ['../README.md'], ['C:/x.md'], ['/x.md']]:
            with self.subTest(files=files), self.assertRaises(ValueError):
                self.findings(files=files)

    def test_read_only_and_independent_cwd_cli(self):
        self.put('tools/check_docs.py', Path(guard.__file__).read_text(encoding='utf-8'))
        self.commit()
        paths = [p for p in self.root.rglob('*') if p.is_file()]
        before = {str(p.relative_to(self.root)): hashlib.sha256(p.read_bytes()).hexdigest() for p in paths}
        env = dict(os.environ, PYTHONDONTWRITEBYTECODE='1')
        result = subprocess.run([sys.executable, '-B', str(self.root / 'tools/check_docs.py')],
                                cwd=guard.ROOT, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertIn(b'RESULT=PASSED', result.stdout)
        after = {str(p.relative_to(self.root)): hashlib.sha256(p.read_bytes()).hexdigest()
                 for p in self.root.rglob('*') if p.is_file()}
        self.assertEqual(before, after)


    def test_task_progress_shapes_require_declaration_and_keep_full_differences(self):
        shapes = [
            ('- [ ] T029 original obligation', '- [x] T029 original obligation'),
            ('- [ ] T018/H02 planned local slice', '- [x] T018/H02 scoped result; joint remains unverified'),
            ('  - [ ] **H02** remaining product doors', '  - [x] **H02** scoped doors closed; broader doors remain'),
            ('- [x] handoff: 90 minute window pending ACK', '- [x] handoff: user cancelled window; input checks retained'),
        ]
        for old, new in shapes:
            with self.subTest(old=old):
                self.put('specs/a/tasks.md', old + '\n')
                self.commit()
                self.put('specs/a/tasks.md', new + '\n')
                base = self.git('rev-parse', 'HEAD').decode().strip()
                self.assert_error('changed-checkbox-line', base=base)
                self.assert_error('task-update-review-required', base=base, mode='development')
                commit, scope, findings = guard.audit(self.root, base=base, mode='development', reviewed_task_updates=True)
                self.assertEqual(commit, base)
                self.assertEqual(scope, ['README.md', 'specs/a/tasks.md'])
                self.assertFalse([f for f in findings if f[0] == 'ERROR'])
                removed = [f for f in findings if f[2] == 'task-line-removed']
                added = [f for f in findings if f[2] == 'task-line-added']
                self.assertEqual(len(removed), 1)
                self.assertEqual(len(added), 1)
                self.assertEqual(removed[0][3], old + ' (count=1; caller declaration is not authorization/acceptance proof)')
                self.assertEqual(added[0][3], new + ' (count=1; caller declaration is not authorization/acceptance proof)')
                self.assertTrue(all(f[0] == 'REVIEW' for f in removed + added))

    def test_development_unchanged_tasks_need_no_declaration(self):
        self.put('specs/a/tasks.md', '- [ ] T001 stable\n')
        self.commit()
        self.assertEqual(self.findings(mode='development'), [])

    def test_development_added_anonymous_task_also_needs_declaration(self):
        self.put('specs/a/tasks.md', '- [ ] anonymous new obligation\n')
        self.assert_error('task-update-review-required', mode='development')
        findings = self.findings(mode='development', reviewed_task_updates=True)
        self.assertEqual([f[2] for f in findings if f[0] == 'REVIEW'], ['task-line-added'])
        self.assertFalse([f for f in findings if f[0] == 'ERROR'])

    def test_development_duplicate_and_anonymous_deletions_are_not_paired(self):
        old = '  - [ ] **H02** stable local scope'
        new = '  - [x] **H02** reviewed local result'
        self.put('specs/a/tasks.md', (old + '\n') * 3)
        self.commit()
        self.put('specs/a/tasks.md', old + '\n' + new + '\n')
        findings = self.findings(mode='development', reviewed_task_updates=True)
        self.assertFalse([f for f in findings if f[0] == 'ERROR'])
        review = [f for f in findings if f[0] == 'REVIEW']
        self.assertEqual(len(review), 2)
        self.assertTrue(any(f[2] == 'task-line-removed' and f[3].startswith(old + ' (count=2;') for f in review))
        self.assertTrue(any(f[2] == 'task-line-added' and f[3].startswith(new + ' (count=1;') for f in review))

    def test_declared_development_preserves_id_anchor_and_link_errors(self):
        self.put('specs/a/tasks.md', '# Stable\n- [ ] T001 obligation\n')
        self.commit()
        self.put('specs/a/tasks.md', '# Changed\n- [x] anonymous [bad](missing.md)\n')
        kinds = {f[2] for f in self.errors(mode='development', reviewed_task_updates=True)}
        self.assertTrue({'removed-id', 'removed-anchor', 'missing-link'}.issubset(kinds))

    def test_api_invalid_mode_and_maintenance_declaration_fail_closed(self):
        with self.assertRaisesRegex(ValueError, 'invalid mode'):
            self.findings(mode='progress')
        with self.assertRaisesRegex(ValueError, 'requires --mode development'):
            self.findings(reviewed_task_updates=True)

    def test_cli_task_declaration_exit_codes_and_semantic_boundary(self):
        self.put('specs/a/tasks.md', '- [ ] T029 same obligation\n')
        self.commit()
        self.put('specs/a/tasks.md', '- [x] T029 same obligation\n')
        for argv, expected in [([], 1), (['--mode', 'development'], 1),
                               (['--mode', 'development', '--reviewed-task-updates'], 0),
                               (['--reviewed-task-updates'], 2)]:
            with self.subTest(argv=argv):
                output = io.StringIO()
                with patch.object(guard, 'ROOT', self.root), redirect_stdout(output):
                    code = guard.main(argv)
                self.assertEqual(code, expected)
                text = output.getvalue()
                if '--mode' in argv:
                    self.assertIn('task-line-removed', text)
                    self.assertIn('task-line-added', text)
                    self.assertIn('task_difference_rows=2', text)
                    self.assertIn('no maintenance pass or semantic/authorization/acceptance verification is claimed', text)
                    self.assertIn('not authorization/acceptance proof', text)
                if expected == 0:
                    self.assertIn('TASK_UPDATE_DECLARATION=caller-declared; NOT verified', text)
                    self.assertIn('RESULT=PASSED', text)


if __name__ == '__main__':
    unittest.main(verbosity=2)
