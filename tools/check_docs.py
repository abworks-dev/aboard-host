#!/usr/bin/env python3
"""Read-only Git-baseline Markdown guard (Python 3 stdlib + Git).

Supported subset: ATX/setext headings, single-line HTML a/id anchors, inline
Markdown links/images and reference links, fenced/indented code and inline code.
Not a CommonMark renderer: HTML links, includes, MDX, multiline destinations,
nested link labels, escaped delimiters and renderer-specific slugs need review.
No network, repair, authorization, acceptance or business-status inference.
"""
import argparse
from collections import Counter
from dataclasses import dataclass, field
import html
from pathlib import Path, PurePosixPath
import re
import stat
import subprocess
import sys
from urllib.parse import unquote, urlsplit

ROOT = Path(__file__).resolve().parents[1]
EXCLUDED = {'.data', 'delivery', 'node_modules', 'vendor', '.git', 'history',
            'historical', 'experiments', 'archive', 'archived', '__pycache__'}
TASK = re.compile(r'^\s*[-*+] \[[ xX]\]\s+')
ANCHOR = re.compile(r'<(?:a|[a-z][\w-]*)\b[^>]*?\b(?:id|name)\s*=\s*[\"\']([^\"\']+)[\"\'][^>]*>', re.I)
INLINE = re.compile(r'(`+)(.*?)\1')
LINK = re.compile(r'!?\[[^\]\n]*\]\(\s*(?:<([^>\n]+)>|([^\s)]+))(?:\s+[\"\'][^\n]*?[\"\'])?\s*\)')
REFDEF = re.compile(r'^\s{0,3}\[([^\]]+)\]:\s*(?:<([^>]+)>|(\S+))\s*(?:[\"\'].*)?$')
REFUSE = re.compile(r'!?\[([^\]\n]+)\](?:\[([^\]\n]*)\])?(?!\()')


def git(root, *args, check=True):
    result = subprocess.run(['git', '-C', str(root), *args], stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE)
    if check and result.returncode:
        raise ValueError('Git failed: ' + result.stderr.decode('utf-8', 'replace').strip())
    return result


def maintained(path):
    p = PurePosixPath(path)
    return (p.suffix.lower() == '.md' and not (set(p.parts) & EXCLUDED)
            and (path in {'AGENTS.md', 'README.md', '.specify/memory/constitution.md'}
                 or p.parts[0] in {'docs', 'specs'}))


def names(data):
    return {n.decode('utf-8') for n in data.split(b'\0') if n}


def safe_path(root, name):
    p = root / name
    # Never follow reparse points, including repository-internal links to ignored
    # evidence. lstat each component before resolve/exists/read touches its target.
    component = root
    for part in PurePosixPath(name).parts:
        component = component / part
        try:
            info = component.lstat()
        except FileNotFoundError:
            break
        if (stat.S_ISLNK(info.st_mode) or
                getattr(info, 'st_file_attributes', 0) & getattr(stat, 'FILE_ATTRIBUTE_REPARSE_POINT', 0x400)):
            raise ValueError('symlink/reparse boundary: ' + name)
    if not p.resolve().is_relative_to(root.resolve()):
        raise ValueError('outside repository: ' + name)
    return p


def slug(text):
    text = re.sub(r'<[^>]+>', '', html.unescape(text)).strip().lower()
    text = re.sub(r'[^\w\-\s]', '', text, flags=re.UNICODE).replace('_', '_')
    return re.sub(r'\s', '-', text)


def visible_lines(text):
    """Mask comments/code without changing line numbers; keep inline-code content separate."""
    text = re.sub(r'<!--[\s\S]*?-->', lambda m: re.sub(r'[^\n]', ' ', m[0]), text)
    fence = None
    result = []
    for raw in text.splitlines():
        opening = re.match(r'^ {0,3}(`{3,}|~{3,})(.*)$', raw)
        if fence:
            if re.match(r'^ {0,3}' + re.escape(fence[0]) + '{' + str(fence[1]) + r',}\s*$', raw):
                fence = None
            result.append('')
        elif opening:
            fence = (opening[1][0], len(opening[1]))
            result.append('')
        elif raw.startswith('    ') or raw.startswith('\t'):
            result.append('')
        else:
            result.append(raw)
    return result


@dataclass
class Document:
    anchors: Counter = field(default_factory=Counter)
    headings: Counter = field(default_factory=Counter)
    explicit: Counter = field(default_factory=Counter)
    ids: Counter = field(default_factory=Counter)
    checkboxes: Counter = field(default_factory=Counter)
    links: list = field(default_factory=list)
    unverified: list = field(default_factory=list)


def parse(text, path):
    doc = Document()
    lines = visible_lines(text)
    refs = {}
    for raw in lines:
        line = INLINE.sub('', raw)
        match = REFDEF.match(line)
        if match:
            refs[match[1].casefold()] = match[2] or match[3]
    table_definitions = False
    is_tasks = PurePosixPath(path).name == 'tasks.md'
    for index, raw in enumerate(lines):
        line = INLINE.sub('', raw)
        for anchor in ANCHOR.findall(line):
            doc.explicit[html.unescape(anchor)] += 1
        heading = re.match(r'^ {0,3}#{1,6}\s+(.+?)\s*#*\s*$', raw)
        title = heading[1] if heading else None
        if not title and index + 1 < len(lines) and raw.strip() and re.match(r'^ {0,3}(?:=+|-+)\s*$', lines[index + 1]):
            title = raw.strip()
        if title:
            base = slug(title)
            n = doc.headings[base]
            doc.anchors[base + (('-' + str(n)) if n else '')] += 1
            doc.headings[base] += 1
            story = re.match(r'(?:\*\*)?(US\d+)\b|User Story (\d+)\b', title)
            if story:
                doc.ids[story[1] or ('US' + story[2])] += 1
        # Definitions, not every token: stories in headings; FR/SC bullets;
        # T checkbox entries; V/G only tasks.md definition-shaped headings/bullets
        # or tables whose first header cell is explicitly a gate/verification ID.
        definition = re.match(r'^\s*[-*+]\s+(?:\*\*)?((?:FR|SC)-\d+)\b', line)
        task = re.match(r'^\s*[-*+] \[[ xX]\]\s+(T\d+)\b', line)
        if definition:
            doc.ids[definition[1]] += 1
        if is_tasks:
            if task:
                doc.ids[task[1]] += 1
            gate = re.match(r'(?:\*\*)?([VG]\d+[a-z]?)\b', title or '')
            bullet = re.match(r'^\s*[-*+]\s+(?:\*\*)?([VG]\d+[a-z]?)\b', line)
            if gate or bullet:
                doc.ids[(gate or bullet)[1]] += 1
            if line.lstrip().startswith('|'):
                first_cell = line.strip().strip('|').split('|')[0].strip()
                if re.match(r'^(?:门|验证|V\s*/|ID\b|Gate\b|Verification\b)', first_cell):
                    table_definitions = True
                table_id = re.match(r'([VG]\d+[a-z]?)\b', first_cell)
                if table_definitions and table_id:
                    doc.ids[table_id[1]] += 1
            else:
                table_definitions = False
            if TASK.match(line):
                doc.checkboxes[raw] += 1
        if REFDEF.match(line):
            continue
        spans = []
        for link in LINK.finditer(line):
            doc.links.append((index + 1, html.unescape(link[1] or link[2])))
            spans.append(link.span())
        remaining = list(line)
        for start, end in spans:
            remaining[start:end] = ' ' * (end - start)
        for ref in REFUSE.finditer(''.join(remaining)):
            key = (ref[2] or ref[1]).casefold()
            if key in refs:
                doc.links.append((index + 1, html.unescape(refs[key])))
            elif ref[2] is not None:
                doc.unverified.append((index + 1, 'undefined reference: ' + key))
    doc.anchors.update(doc.explicit)
    return doc


class Snapshot:
    def __init__(self, root, commit=None):
        self.root, self.commit = root, commit
        if commit:
            self.paths = names(git(root, 'ls-tree', '-r', '-z', '--name-only', commit).stdout)
        else:
            self.paths = names(git(root, 'ls-files', '-z', '--cached', '--others', '--exclude-standard').stdout)
        self.directories = {str(parent) for name in self.paths
                            for parent in PurePosixPath(name).parents if str(parent) != '.'}
        self.cache = {}
        self.ignored = {}

    def excluded(self, name):
        if set(PurePosixPath(name).parts) & EXCLUDED:
            return True
        if name not in self.ignored:
            self.ignored[name] = git(self.root, 'check-ignore', '--no-index', '-q', '--', name, check=False).returncode == 0
        return self.ignored[name]

    def text(self, name):
        if name not in self.cache:
            if self.excluded(name):
                raise ValueError('excluded document: ' + name)
            if self.commit:
                blob = git(self.root, 'show', self.commit + ':' + name, check=False)
                self.cache[name] = blob.stdout.decode('utf-8-sig') if blob.returncode == 0 else None
            else:
                p = safe_path(self.root, name)
                self.cache[name] = p.read_text(encoding='utf-8-sig') if p.is_file() else None
        return self.cache[name]

    def link_issue(self, source, dest):
        if re.match(r'^[A-Za-z]:[\\/]', dest) or dest.startswith(('\\', '/')):
            return ('boundary', dest)
        parts = urlsplit(dest)
        if parts.scheme or parts.netloc:
            return ('boundary', dest)
        name = unquote(parts.path)
        if '\\' in name:
            return ('boundary', dest)
        target = PurePosixPath(source).parent / name if name else PurePosixPath(source)
        # Lexical collapse first; never touch an outside path even to test existence.
        stack = []
        for part in target.parts:
            if part == '..':
                if not stack:
                    return ('boundary', dest)
                stack.pop()
            elif part != '.':
                stack.append(part)
        target = '/'.join(stack)
        if self.excluded(target):
            return ('boundary', dest)
        try:
            p = safe_path(self.root, target)
        except ValueError:
            return ('boundary', dest)
        exists = (target in self.paths or target in self.directories or target == '') if self.commit else p.exists()
        if not exists:
            return ('missing-link', dest)
        fragment = unquote(parts.fragment)
        if re.fullmatch(r'L\d+(?:-L?\d+)?', fragment):
            return ('boundary', dest)  # Existence only; no source-line/range validation.
        if fragment and PurePosixPath(target).suffix.lower() == '.md':
            text = self.text(target)
            if text is None or fragment not in parse(text, target).anchors:
                return ('missing-fragment', dest)
        elif fragment:
            return ('boundary', dest)  # Source line/renderer anchors are not Markdown headings.
        return None


def audit(root, base='HEAD', files=None, mode='maintenance', reviewed_task_updates=False):
    if mode not in ('maintenance', 'development'):
        raise ValueError('invalid mode: ' + str(mode))
    if reviewed_task_updates and mode != 'development':
        raise ValueError('--reviewed-task-updates requires --mode development; maintenance protection is unchanged')
    root = root.resolve()
    # --end-of-options prevents option injection; a bad explicit baseline is fatal, never fallback.
    commit = git(root, 'rev-parse', '--verify', '--end-of-options', base + '^{commit}').stdout.decode().strip()
    before, current = Snapshot(root, commit), Snapshot(root)
    scope = {p for p in before.paths | current.paths if maintained(p) and not current.excluded(p)}
    if files is not None:
        chosen = set()
        for raw in files:
            name = raw.replace('\\', '/')
            if name.startswith('/') or re.match(r'^[A-Za-z]:', name) or '..' in PurePosixPath(name).parts:
                raise ValueError('--files requires repository-relative maintained Markdown: ' + raw)
            name = str(PurePosixPath(name))
            if name not in scope:
                raise ValueError('--files outside discovered maintained scope: ' + raw)
            chosen.add(name)
        scope = chosen
    if not scope:
        raise ValueError('empty maintained scope; no PASS')
    findings = []
    def emit(level, path, kind, detail):
        findings.append((level, path, kind, detail))
    for path in sorted(scope):
        try:
            safe_path(root, path)
        except ValueError as exc:
            emit('UNVERIFIED', path, 'source-boundary', str(exc))
            continue
        old_text, new_text = before.text(path), current.text(path)
        old, new = parse(old_text or '', path), parse(new_text or '', path)
        if old_text is None:
            emit('INFO', path, 'new-document', 'not present in baseline (including untracked historical text)')
        for anchor in sorted(old.anchors.keys() - new.anchors.keys()):
            emit('ERROR', path, 'removed-anchor', anchor)
        for kind, previous, now in [('duplicate-anchor', old.explicit, new.explicit),
                                    ('duplicate-heading', old.headings, new.headings),
                                    ('duplicate-id', old.ids, new.ids)]:
            for key, count in sorted(now.items()):
                if count > 1:
                    emit('ERROR' if count > max(1, previous[key]) else 'WARN', path, kind, key + ' count=' + str(count))
        for key in sorted(old.ids.keys() - new.ids.keys()):
            emit('ERROR', path, 'removed-id', key)
        removed = old.checkboxes - new.checkboxes
        added = new.checkboxes - old.checkboxes
        if mode == 'maintenance':
            for line, count in removed.items():
                emit('ERROR', path, 'changed-checkbox-line', line + ' (removed/changed count=' + str(count) + ')')
        elif removed or added:
            # A caller declaration, NOT inferred authorization or semantic validation.
            # Anonymous/duplicate rows cannot be reliably paired: retain multiset counts.
            if not reviewed_task_updates:
                emit('ERROR', path, 'task-update-review-required',
                     'development task differences require --reviewed-task-updates after existing authorization and semantic review')
            for side, changes in [('removed', removed), ('added', added)]:
                for line, count in sorted(changes.items()):
                    emit('REVIEW', path, 'task-line-' + side,
                         line + ' (count=' + str(count) + '; caller declaration is not authorization/acceptance proof)')
        for line, detail in new.unverified:
            emit('UNVERIFIED', path, 'markdown-subset', str(line) + ': ' + detail)
        old_issues = Counter(before.link_issue(path, dest) for _, dest in old.links)
        for line, dest in new.links:
            issue = current.link_issue(path, dest)
            if issue is None:
                continue
            if issue[0] == 'boundary':
                emit('UNVERIFIED', path, 'boundary', str(line) + ': ' + dest)
            elif old_issues[issue]:
                old_issues[issue] -= 1
                emit('WARN', path, issue[0], str(line) + ': ' + dest + ' (baseline debt)')
            else:
                emit('ERROR', path, issue[0], str(line) + ': ' + dest)
    return commit, sorted(scope), findings


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', default='HEAD', help='explicit Git commit/ref; invalid is fatal (default HEAD)')
    parser.add_argument('--files', nargs='+', help='restrict source documents; repository-relative, nonempty; NOT a full audit')
    parser.add_argument('--mode', choices=['maintenance', 'development'], default='maintenance',
                        help='development checks structure, not task commitments/authorization/acceptance; task differences require a caller declaration')
    parser.add_argument('--reviewed-task-updates', action='store_true',
                        help='development only: declare all task differences already authorized and semantically reviewed; not approval or proof')
    args = parser.parse_args(argv)
    try:
        commit, scope, findings = audit(ROOT, args.base, args.files, args.mode, args.reviewed_task_updates)
    except (ValueError, OSError, UnicodeError) as exc:
        print('FAILED: ' + str(exc))
        return 2
    print('ROOT=' + str(ROOT) + '\nBASE=' + commit + '\nMODE=' + args.mode)
    print('TASK_UPDATE_DECLARATION=' + ('caller-declared; NOT verified authorization/acceptance' if args.reviewed_task_updates else 'none'))
    print('SCOPE=' + ('LIMITED (--files), NOT whole repository' if args.files else 'maintained Markdown only; NOT all repository files'))
    for path in scope:
        print('FILE ' + path)
    for level, path, kind, detail in findings:
        print(level + ' ' + path + ' [' + kind + '] ' + detail)
    counts = Counter(f[0] for f in findings)
    print('RESULT=' + ('FAILED' if counts['ERROR'] else 'PASSED') + ' documents=' + str(len(scope)) +
          ' errors=' + str(counts['ERROR']) + ' warnings=' + str(counts['WARN']) +
          ' unverified=' + str(counts['UNVERIFIED']) + ' task_difference_rows=' + str(counts['REVIEW']))
    if args.mode == 'development':
        print('DEVELOPMENT STRUCTURE ONLY: task commitments are not automatically protected; no maintenance pass or semantic/authorization/acceptance verification is claimed.')
    print('Static subset only; no repair/network/authorization/business acceptance. Semantic review remains required.')
    return 1 if counts['ERROR'] else 0


if __name__ == '__main__':
    sys.exit(main())
