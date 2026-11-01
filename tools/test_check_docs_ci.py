#!/usr/bin/env python3
"""Stdlib CI-helper tests: synthetic Git fixtures only; never mutate real Git.

Retain unique fixtures under this repository's ignored CI evidence directory.
No network, dependency install, Host/native/browser, or business test execution.
"""
import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
import check_docs_ci as ci


class SelectionTests(unittest.TestCase):
    def test_pr_uses_base_sha_not_head_or_before(self):
        self.assertEqual(ci.select_base('pull_request', {
            'pull_request': {'base': {'sha': 'A' * 40}, 'head': {'sha': 'b' * 40}},
            'before': 'c' * 40}), 'a' * 40)

    def test_push_uses_before_not_after_or_pr(self):
        self.assertEqual(ci.select_base('push', {'before': 'b' * 40,
                                               'after': 'c' * 40}), 'b' * 40)

    def test_dispatch_uses_explicit_input_not_before(self):
        self.assertEqual(ci.select_base('workflow_dispatch', {
            'inputs': {'base_sha': 'd' * 40}, 'before': 'e' * 40}), 'd' * 40)

    def test_missing_fields_never_fall_back(self):
        for name, payload in [('pull_request', {'before': 'a' * 40}),
                              ('pull_request', {'pull_request': {'base': {}}}),
                              ('push', {'after': 'a' * 40}),
                              ('workflow_dispatch', {'before': 'a' * 40}),
                              ('workflow_dispatch', {'inputs': {}})]:
            with self.subTest(name=name, payload=payload), self.assertRaises(ValueError):
                ci.select_base(name, payload)

    def test_zero_for_each_event_fails(self):
        for name, payload in [('push', {'before': '0' * 40}),
                              ('pull_request', {'pull_request': {'base': {'sha': '0' * 40}}}),
                              ('workflow_dispatch', {'inputs': {'base_sha': '0' * 40}})]:
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, 'zero baseline'):
                ci.select_base(name, payload)

    def test_bad_sha_ref_short_options_and_shell_text_rejected(self):
        for value in [None, 12, '', 'HEAD', 'HEAD^', 'abc123', '--help',
                      'g' * 40, 'a' * 41, 'a' * 39, ' ' + 'a' * 40,
                      '$(touch marker)', 'a' * 40 + '\n']:
            with self.subTest(value=value), self.assertRaises(ValueError):
                ci.select_base('push', {'before': value})

    def test_nonobject_and_wrong_nested_types_fail(self):
        for name, payload in [('push', []), ('push', None),
                              ('pull_request', {'pull_request': []}),
                              ('pull_request', {'pull_request': {'base': 'a' * 40}}),
                              ('workflow_dispatch', {'inputs': []})]:
            with self.subTest(payload=payload), self.assertRaises(ValueError):
                ci.select_base(name, payload)

    def test_other_events_including_pr_target_rejected(self):
        for name in [None, '', 'pull_request_target', 'schedule', 'push\n::notice::bad']:
            with self.subTest(name=name), self.assertRaises(ValueError):
                ci.select_base(name, {'before': 'a' * 40})

    def test_mode_defaults_and_automatic_events_ignore_spoofed_inputs(self):
        self.assertEqual(ci.select_mode('workflow_dispatch', {}), ('maintenance', False))
        for event in ('push', 'pull_request'):
            for inputs in ({'mode': 'development', 'reviewed_task_updates': 'true'},
                           {'mode': 'invalid', 'reviewed_task_updates': []}):
                self.assertEqual(ci.select_mode(event, {'inputs': inputs}), ('maintenance', False))
        for value in (True, 'true'):
            self.assertEqual(ci.select_mode('workflow_dispatch', {'inputs': {
                'mode': 'development', 'reviewed_task_updates': value}}), ('development', True))
        for value in (False, 'false'):
            self.assertEqual(ci.select_mode('workflow_dispatch', {'inputs': {
                'mode': 'development', 'reviewed_task_updates': value}}), ('development', False))

    def test_invalid_mode_declarations_are_never_truthiness_coerced(self):
        for inputs in ([], {'mode': 'auto'}, {'mode': []},
                       {'reviewed_task_updates': True},
                       *({'mode': 'development', 'reviewed_task_updates': value}
                         for value in (None, 0, 1, [], {}, 'yes', 'TRUE', ''))):
            with self.subTest(inputs=inputs), self.assertRaises(ValueError):
                ci.select_mode('workflow_dispatch', {'inputs': inputs})
        with self.assertRaises(ValueError):
            ci.select_mode('workflow_run', {})

    def test_tag_resolution_not_an_exact_commit_is_rejected(self):
        completed = subprocess.CompletedProcess([], 0, stdout=('b' * 40 + '\n').encode())
        with patch.object(ci.subprocess, 'run', return_value=completed):
            with self.assertRaisesRegex(ValueError, 'not a tag or ref'):
                ci.commit_sha(Path('.'), 'a' * 40)


class IntegrationTests(unittest.TestCase):
    def setUp(self):
        parent = ci.ROOT / '.data/logs/docs-ci-20261029-host-owner/fixtures'
        parent.mkdir(parents=True, exist_ok=True)
        self.root = Path(tempfile.mkdtemp(prefix='case-', dir=parent))
        self.git('init', '-q')
        self.put('.gitignore', '.data/\n__pycache__/\n')
        self.put('README.md', '# Example\n[Target](docs/target.md#stable)\n')
        self.put('docs/target.md', '# Stable\n')
        self.put('specs/example/tasks.md', '# Tasks\n- [ ] T001 preserve this entire line\n')
        for name in ['check_docs.py', 'check_docs_ci.py']:
            self.put('tools/' + name, (ci.ROOT / 'tools' / name).read_text(encoding='utf-8'))
        self.commit()
        self.base = self.git('rev-parse', 'HEAD').decode().strip()
        # A distinct real checked-out commit, without changing maintained docs.
        self.put('fixture-revision.txt', 'synthetic head\n')
        self.commit()
        self.head = self.git('rev-parse', 'HEAD').decode().strip()
        self.event = self.root / '.data/event.json'
        self.write_event({'before': self.base})

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
        self.git('-c', 'user.name=Synthetic CI Fixture',
                 '-c', 'user.email=fixture@example.invalid', '-c', 'commit.gpgsign=false',
                 'commit', '-qm', 'synthetic CI revision')

    def write_event(self, payload):
        self.put('.data/event.json', json.dumps(payload))

    def invoke(self, event_name='push', path=None, use_env=False):
        env = dict(os.environ, PYTHONDONTWRITEBYTECODE='1')
        command = [sys.executable, '-B', str(self.root / 'tools/check_docs_ci.py')]
        if use_env:
            env.update(GITHUB_EVENT_NAME=event_name, GITHUB_EVENT_PATH=str(path or self.event))
        else:
            command += ['--event-name', event_name, '--event-path', str(path or self.event)]
        return subprocess.run(command, cwd=ci.ROOT, env=env,
                              stdout=subprocess.PIPE, stderr=subprocess.PIPE)

    def test_clean_full_maintenance_positive(self):
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        for text in ['BASE=' + self.base, 'HEAD=' + self.head, 'MODE=maintenance',
                     'FILE README.md', 'FILE docs/target.md', 'FILE specs/example/tasks.md',
                     'RESULT=PASSED exit=0']:
            self.assertIn(text.encode(), result.stdout)
        self.assertNotIn(b'--files', result.stdout)
        self.assertNotIn(b'development', result.stdout)

    def test_pr_and_dispatch_run_real_checker(self):
        for name, payload in [
            ('pull_request', {'pull_request': {'base': {'sha': self.base}}}),
            ('workflow_dispatch', {'inputs': {'base_sha': self.base}})]:
            with self.subTest(name=name):
                self.write_event(payload)
                result = self.invoke(name)
                self.assertEqual(result.returncode, 0, result.stderr.decode())
                self.assertIn(('BASE=' + self.base).encode(), result.stdout)

    def test_environment_event_inputs(self):
        result = self.invoke(use_env=True)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertIn(b'EVENT="push"', result.stdout)

    def test_unchanged_source_deleted_target_is_found_globally(self):
        target = (self.root / 'docs/target.md').resolve()
        self.assertTrue(target.is_relative_to(self.root.resolve()))
        target.unlink()
        result = self.invoke()
        self.assertEqual(result.returncode, 1)
        self.assertIn(b'ERROR README.md [missing-link]', result.stdout)
        self.assertIn(b'RESULT=FAILED exit=1', result.stdout)

    def test_maintenance_rejects_task_line_change(self):
        self.put('specs/example/tasks.md', '# Tasks\n- [x] T001 preserve this entire line\n')
        result = self.invoke()
        self.assertEqual(result.returncode, 1)
        self.assertIn(b'changed-checkbox-line', result.stdout)

    def test_dispatch_development_full_task_diff_and_global_structure(self):
        old = '- [ ] T001 preserve this entire line'
        new = '- [x] T001 preserve this entire line'
        self.put('specs/example/tasks.md', '# Tasks\n' + new + '\n')
        self.write_event({'inputs': {'base_sha': self.base, 'mode': 'development',
                                    'reviewed_task_updates': 'true'}})
        before = self.fixture_bytes()
        result = self.invoke('workflow_dispatch')
        self.assertEqual(result.returncode, 0, result.stdout.decode())
        for text in ('BASE=' + self.base, 'MODE=development', old, new, 'REVIEW',
                     '--reviewed-task-updates', 'RESULT=PASSED exit=0'):
            self.assertIn(text.encode(), result.stdout)
        self.assertEqual(before, self.fixture_bytes())
        self.assertNotIn(b'--files', result.stdout)
        self.put('docs/target.md', '# Changed anchor\n')
        before = self.fixture_bytes()
        result = self.invoke('workflow_dispatch')
        self.assertEqual(result.returncode, 1)
        self.assertIn(b'missing-fragment', result.stdout)
        self.assertEqual(before, self.fixture_bytes())

    def test_dispatch_development_without_declaration_fails_on_task_changes(self):
        self.put('specs/example/tasks.md', '# Tasks\n- [x] T001 preserve this entire line\n')
        self.write_event({'inputs': {'base_sha': self.base, 'mode': 'development',
                                    'reviewed_task_updates': 'false'}})
        result = self.invoke('workflow_dispatch')
        self.assertEqual(result.returncode, 1)
        self.assertIn(b'REVIEW', result.stdout)
        self.assertNotIn(b'"--reviewed-task-updates"', result.stdout)

    def test_automatic_events_do_not_upgrade_even_with_spoofed_declaration(self):
        self.put('specs/example/tasks.md', '# Tasks\n- [x] T001 preserve this entire line\n')
        for event, base in (('push', {'before': self.base}),
                            ('pull_request', {'pull_request': {'base': {'sha': self.base}}})):
            with self.subTest(event=event):
                self.write_event(dict(base, inputs={'mode': 'development', 'reviewed_task_updates': True}))
                result = self.invoke(event)
                self.assertEqual(result.returncode, 1)
                self.assertIn(b'MODE=maintenance', result.stdout)
                self.assertNotIn(b'"--reviewed-task-updates"', result.stdout)
                self.assertNotIn(b'MODE=development', result.stdout)

    def test_invalid_dispatch_policy_rejects_before_checker(self):
        self.write_event({'inputs': {'base_sha': self.base, 'reviewed_task_updates': 'true'}})
        result = self.invoke('workflow_dispatch')
        self.assertEqual(result.returncode, 2)
        self.assertNotIn(b'COMMAND=', result.stdout)

    def fixture_bytes(self):
        return {str(p.relative_to(self.root)): p.read_bytes()
                for p in self.root.rglob('*') if p.is_file()}

    def test_unavailable_base_fails_without_checker_or_fallback(self):
        self.write_event({'before': 'f' * 40})
        result = self.invoke()
        self.assertEqual(result.returncode, 2)
        self.assertIn(b'commit unavailable', result.stdout)
        self.assertNotIn(b'COMMAND=', result.stdout)

    def test_self_baseline_fails(self):
        self.write_event({'before': self.head})
        result = self.invoke()
        self.assertEqual(result.returncode, 2)
        self.assertIn(b'self-baseline rejected', result.stdout)
        self.assertNotIn(b'COMMAND=', result.stdout)

    def test_bad_json_fails(self):
        self.put('.data/event.json', '{broken')
        result = self.invoke()
        self.assertEqual(result.returncode, 2)
        self.assertIn(b'RESULT=FAILED exit=2', result.stdout)
        self.assertNotIn(b'COMMAND=', result.stdout)

    def test_missing_event_file_fails(self):
        result = self.invoke(path=self.root / '.data/not-existing.json')
        self.assertEqual(result.returncode, 2)
        self.assertNotIn(b'COMMAND=', result.stdout)

    def test_zero_and_missing_cli_baselines_fail(self):
        for payload in [{'before': '0' * 40}, {}, {'before': 'HEAD'}]:
            with self.subTest(payload=payload):
                self.write_event(payload)
                result = self.invoke()
                self.assertEqual(result.returncode, 2)
                self.assertNotIn(b'COMMAND=', result.stdout)

    def test_missing_checker_exit_two_is_preserved(self):
        target = (self.root / 'tools/check_docs.py').resolve()
        self.assertTrue(target.is_relative_to(self.root.resolve()))
        target.unlink()
        result = self.invoke()
        self.assertEqual(result.returncode, 2)
        self.assertIn(b'RESULT=FAILED exit=2', result.stdout)

    def test_arbitrary_command_failure_code_is_preserved(self):
        real_run = ci.subprocess.run
        def controlled(command, **kwargs):
            if command[0] == sys.executable:
                self.assertEqual(command[-4:], ['--base', self.base, '--mode', 'maintenance'])
                self.assertNotIn('shell', kwargs)
                return subprocess.CompletedProcess(command, 7)
            return real_run(command, **kwargs)
        with patch.object(ci.subprocess, 'run', side_effect=controlled), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(ci.run(self.root, 'push', str(self.event)), 7)
        self.assertIn('RESULT=FAILED exit=7', output.getvalue())

    def test_command_spawn_failure_fails_explicitly(self):
        real_run = ci.subprocess.run
        def controlled(command, **kwargs):
            if command[0] == sys.executable:
                raise OSError('synthetic command unavailable')
            return real_run(command, **kwargs)
        with patch.object(ci.subprocess, 'run', side_effect=controlled), contextlib.redirect_stdout(io.StringIO()) as output:
            self.assertEqual(ci.run(self.root, 'push', str(self.event)), 2)
        self.assertIn('synthetic command unavailable', output.getvalue())

    def test_sources_git_and_event_bytes_are_read_only(self):
        def snapshot():
            return {str(p.relative_to(self.root)): hashlib.sha256(p.read_bytes()).hexdigest()
                    for p in self.root.rglob('*') if p.is_file()}
        before = snapshot()
        result = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(before, snapshot())
        # Failure must be just as read-only as success.
        self.write_event({'before': self.head})
        before = snapshot()
        self.assertEqual(self.invoke().returncode, 2)
        self.assertEqual(before, snapshot())


if __name__ == '__main__':
    unittest.main(verbosity=2)
