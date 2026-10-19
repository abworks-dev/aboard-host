"""Independent stdlib oracle for data-model.md §4, NOT Go output.

--write initially freezes synthetic raw JSON bytes + explicit preimage + SHA-256.
Normal execution is read-only and recomputes every expected value independently.
Layout source: specs/001-host-workspace-foundation/data-model.md lines 53-74:
ASCII domain+00; U=uint64 BE <=2^63-1; S=U(UTF8 length)+unchanged UTF8;
UUID=16 display-order hex bytes; D=32 decoded hex bytes; optional=00/01+value.
Public validation schemas are read only from contracts/openapi.json.
This checks byte/structure mechanics, not authorization/commit/key ownership.
"""
from __future__ import annotations
import argparse
from decimal import Decimal
import hashlib
import json
from pathlib import Path
import re
import struct
import sys

ROOT = Path(__file__).resolve().parents[2]
TARGET = ROOT / 'testdata/contracts/digest-vectors.json'
SCHEMAS = json.loads((ROOT / 'contracts/openapi.json').read_text(encoding='utf-8'))['components']['schemas']
MAX = 2**63-1


def require(ok, why):
    if not ok:
        raise ValueError(why)


def unique(pairs):
    obj = {}
    for k, v in pairs:
        require(k not in obj, 'duplicate decoded key')
        obj[k] = v
    return obj


def parse(raw):
    require(len(raw) <= 1048576 and not raw.startswith(b'\xef\xbb\xbf'), 'size/BOM')
    value = json.loads(raw.decode('utf-8', errors='strict'), object_pairs_hook=unique, parse_float=Decimal,
                       parse_constant=lambda _: (_ for _ in ()).throw(ValueError('non-JSON number')))
    def strings(v):
        if isinstance(v, str):
            v.encode('utf-8', errors='strict')
            require('\0' not in v, 'NUL')
        elif isinstance(v, dict):
            for k, val in v.items():
                strings(k)
                strings(val)
        elif isinstance(v, list):
            for val in v:
                strings(val)
    strings(value)
    return value


def matches(s, v):
    if '$ref' in s and not matches(SCHEMAS[s['$ref'].split('/')[-1]], v):
        return False
    if 'const' in s and s['const'] != v:
        return False
    if 'enum' in s and v not in s['enum']:
        return False
    if 'not' in s and matches(s['not'], v):
        return False
    if any(not matches(a, v) for a in s.get('allOf', [])):
        return False
    if 'anyOf' in s and not any(matches(a, v) for a in s['anyOf']):
        return False
    if 'if' in s:
        branch = 'then' if matches(s['if'], v) else 'else'
        if branch in s and not matches(s[branch], v):
            return False
    ty = s.get('type')
    if ty and not {'string': isinstance(v, str), 'object': isinstance(v, dict),
                   'array': isinstance(v, list), 'integer': type(v) is int or (isinstance(v, Decimal) and v == v.to_integral_value()),
                   'boolean': type(v) is bool}.get(ty, False):
        return False
    if type(v) is int or isinstance(v, Decimal):
        if v < s.get('minimum', v) or v > s.get('maximum', v):
            return False
    if isinstance(v, str):
        if len(v) < s.get('minLength', 0) or len(v) > s.get('maxLength', sys.maxsize):
            return False
        if 'pattern' in s and re.search(s['pattern'], v) is None:
            return False
        if len(v.encode('utf-8')) > s.get('x-max-utf8-bytes', sys.maxsize):
            return False
        if 'x-maximum-decimal' in s and int(v) > int(s['x-maximum-decimal']):
            return False
    if isinstance(v, dict):
        if not set(s.get('required', [])) <= v.keys():
            return False
        props = s.get('properties', {})
        if s.get('additionalProperties') is False and not v.keys() <= props.keys():
            return False
        if any(not matches(props[k], val) for k, val in v.items() if k in props):
            return False
    if isinstance(v, list) and 'items' in s and any(not matches(s['items'], x) for x in v):
        return False
    return True


def scalar(schema, value):
    require(matches(SCHEMAS[schema], value), 'invalid ' + schema)
    return value


def u(n):
    require(type(n) is int and 0 <= n <= MAX, 'U overflow/type')
    return struct.pack('>Q', n)


def revision(s, positive=False):
    scalar('PositiveRevision' if positive else 'Revision', s)
    return u(int(s))


def text(s):
    require(isinstance(s, str) and '\0' not in s and not s.startswith('\ufeff'), 'invalid S')
    raw = s.encode('utf-8', errors='strict')
    return u(len(raw)) + raw


def uuid(s):
    return bytes.fromhex(scalar('UUID', s).replace('-', ''))


def digest(s):
    return bytes.fromhex(scalar('Digest', s))


def preimage(kind, v):
    layouts = {
        'identity': ('ABOARD/IDENTITY/1', ['formatVersion','workspaceId','tenantId','databaseRelativePath']),
        'content': ('ABOARD/TEXT/1', ['workspaceId','resourceId','title','body']),
        'candidate': ('ABOARD/CANDIDATE/1', ['workspaceId','principalId','candidateId','resourceId','kind','baseRevision','baseDigest','contentDigest','previewVersion']),
        'save': ('ABOARD/SAVE/1', ['installationId','tenantId','principalId','workspaceId','operationId','candidateId','candidateDigest']),
        'install': ('ABOARD/INSTALL-OP/1', ['installationId','principalId','operationId','kind','workspaceId','canonicalLocator','registrationVersion']),
    }
    domain, fields = layouts[kind]
    optional = {'baseDigest','registrationVersion'}
    require(isinstance(v, dict) and not (v.keys()-set(fields)) and set(fields)-optional <= v.keys(), 'internal layout fields')
    if kind == 'identity':
        require(v['databaseRelativePath'] == 'data/workspace.sqlite', 'database path')
    if kind == 'content':
        scalar('Title', v['title'])
        scalar('Body', v['body'])
    if kind == 'candidate':
        require(v['kind'] in ('create-text','update-text'), 'candidate kind')
        require(('baseDigest' in v) == (v['kind'] == 'update-text'), 'baseDigest presence')
    if kind == 'install':
        require(v['kind'] in ('create','register','relink') and v['canonicalLocator'], 'install kind/locator')
    result = domain.encode('ascii') + b'\x00'
    for f in fields:
        if f in optional:
            result += b'\x01' if f in v else b'\x00'
            if f not in v:
                continue
        val = v[f]
        if f.endswith('Id'):
            result += uuid(val)
        elif f.endswith('Digest'):
            result += digest(val)
        elif f == 'formatVersion':
            result += u(val)
        elif f in ('baseRevision','previewVersion','registrationVersion'):
            result += revision(val, f == 'previewVersion')
        else:
            result += text(val)
    return result


def seeds():
    ids = {key: f'{i:08x}-4455-4677-8899-aabbccddeeff' for i, key in enumerate(
        ['workspaceId','tenantId','resourceId','principalId','candidateId','installationId','operationId'], start=1)}
    ids['workspaceId'] = '00112233-4455-4677-8899-aabbccddeeff'
    d0, d1 = '00'*32, '0123456789abcdef'*4
    vals = []
    def add(label, kind, v, raw=None):
        raw = raw if raw is not None else json.dumps(v, ensure_ascii=False, separators=(',', ':')).encode('utf-8')
        stream = preimage(kind, parse(raw))
        vals.append(dict(name=label, kind=kind, input_hex=raw.hex(), expected_stream_hex=stream.hex(), expected_sha256=hashlib.sha256(stream).hexdigest()))
    identity = dict(formatVersion=2, workspaceId=ids['workspaceId'], tenantId=ids['tenantId'], databaseRelativePath='data/workspace.sqlite')
    add('identity-display-order', 'identity', identity)
    content = dict(workspaceId=ids['workspaceId'], resourceId=ids['resourceId'], title='ASCII', body='')
    add('ascii-empty-body','content',content)
    for label, title, body in [('unicode', '中文😀e\u0301', '正文🧪\n组合e\u0301'),
                               ('lf','换行','line1\nline2'), ('crlf','换行','line1\r\nline2'),
                               ('nfc','é','x'), ('nfd','e\u0301','x')]:
        add(label, 'content', dict(content, title=title, body=body))
    add('json-reordered','content',content, json.dumps(dict(reversed(list(content.items()))), indent=2).encode('utf-8'))
    add('json-escaped','content',content, ('{"body":"","title":"\\u0041SCII","resourceId":"'+ids['resourceId']+'","workspaceId":"'+ids['workspaceId']+'"}').encode())
    candidate = {k:ids[k] for k in ('workspaceId','principalId','candidateId','resourceId')}
    candidate.update(kind='create-text', baseRevision='0', contentDigest=d1, previewVersion='1')
    add('candidate-absence','candidate',candidate)
    add('candidate-2p53plus1','candidate',dict(candidate,kind='update-text',baseRevision='9007199254740993',baseDigest=d0))
    add('candidate-int64max','candidate',dict(candidate,kind='update-text',baseRevision=str(MAX),baseDigest=d1,previewVersion=str(MAX)))
    save = {k:ids[k] for k in ('installationId','tenantId','principalId','workspaceId','operationId','candidateId')}
    save['candidateDigest'] = vals[-1]['expected_sha256']
    add('save-full-binding','save',save)
    install = {k:ids[k] for k in ('installationId','principalId','operationId','workspaceId')}
    install.update(kind='create',canonicalLocator='E:\\合成😀\\workspace')
    add('install-absence','install',install)
    add('install-present-zero','install',dict(install,registrationVersion='0'))
    add('install-int64max','install',dict(install,kind='relink',registrationVersion=str(MAX)))
    negatives = []
    def bad(label, kind=None, value=None, schema=None, raw=None):
        if raw is None:
            raw = json.dumps(value, ensure_ascii=True, separators=(',', ':')).encode()
        entry = dict(name=label, input_hex=raw.hex())
        if schema:
            entry['schema'] = schema
        else:
            entry['kind'] = kind
        negatives.append(entry)
    bad('revision-overflow','candidate',dict(candidate,baseRevision=str(MAX+1)))
    bad('revision-number','candidate',dict(candidate,baseRevision=9007199254740993))
    bad('revision-leading-zero','candidate',dict(candidate,baseRevision='01'))
    bad('zero-preview','candidate',dict(candidate,previewVersion='0'))
    bad('absence-not-empty','candidate',dict(candidate,baseDigest=''))
    bad('update-needs-base','candidate',dict(candidate,kind='update-text'))
    bad('forged-digest','save',dict(save,candidateDigest='g'*64))
    bad('uppercase-digest','save',dict(save,candidateDigest='AB'*32))
    bad('uuid-uppercase','content',dict(content,workspaceId=ids['workspaceId'].upper()))
    bad('uuid-guid-order','content',dict(content,workspaceId='33221100-5544-7746-9988-ffeeddccbbaa'))
    bad('uuid-wrong-version','content',dict(content,resourceId='00000003-4455-1677-8899-aabbccddeeff'))
    bad('identity-u-overflow','identity',dict(identity,formatVersion=MAX+1))
    bad('identity-wrong-path','identity',dict(identity,databaseRelativePath='data/other.sqlite'))
    bad('nul-title','content',dict(content,title='a\0b'))
    bad('bom-text','content',dict(content,title='\ufeffA'))
    bad('title-too-many-scalars','content',dict(content,title='😀'*257))
    bad('body-too-many-utf8-bytes','content',dict(content,body='中'*21846))
    bad('unknown-internal-field','content',dict(content,extra=1))
    bad('invalid-utf8',kind='content',raw=json.dumps(content,separators=(',', ':')).encode().replace(b'ASCII',b'\xff'))
    for label, raw in [('unpaired-high',b'"\\ud800"'),('unpaired-low',b'"\\udc00"'),
                       ('bad-pair',b'"\\ud800\\u0041"'), ('overlong-utf8',b'"\xc0\x80"'),
                       ('utf8-surrogate',b'"\xed\xa0\x80"'), ('utf8-truncated',b'"\xf0\x9f"'),
                       ('bom-json',b'\xef\xbb\xbf"x"'),('nul-escaped',b'"\\u0000"'),
                       ('nul-raw',b'"\x00"'), ('trailing-json',b'"x" "y"')]:
        bad(label,schema='Body',raw=raw)
    bad('duplicate-decoded-key',schema='Diff',raw=b'{"afterTitle":"a","afterBody":"b","after\\u0042ody":"c"}')
    bad('unknown-field',schema='Diff',value=dict(afterTitle='a',afterBody='b',other='x'))
    bad('casefold-field',schema='Diff',value=dict(afterTitle='a',AfterBody='b'))
    bad('nested-unknown',schema='CandidateView',value=dict(validation=dict(state='valid',hidden='x')))
    for label, val in [('numeric',9007199254740993),('overflow',str(MAX+1)),('leading-zero','01'),('negative','-1'),('exponent','1e3')]:
        bad('public-revision-'+label,schema='Revision',value=val)
    bad('null-optional',schema='Diff',value=dict(afterTitle='a',afterBody='b',beforeBody=None))
    preview=dict(openEpoch=ids['workspaceId'],kind='create-text',baseRevision='0',title='a',body='')
    bad('create-forbids-resource',schema='PreviewRecord',value=dict(preview,resourceId=ids['resourceId']))
    bad('create-forbids-base',schema='PreviewRecord',value=dict(preview,baseDigest=d0))
    bad('update-needs-fields',schema='PreviewRecord',value=dict(preview,kind='update-text'))
    return dict(format=1, source='data-model.md §4; independent Python struct.pack/bytes.fromhex/hashlib; no Go output', vectors=vals, rejected=negatives)


def verify(doc):
    require(doc['format'] == 1 and 'no Go output' in doc['source'], 'fixture provenance')
    names = set()
    for row in doc['vectors']:
        require(row['name'] not in names, 'duplicate vector name')
        names.add(row['name'])
        raw = bytes.fromhex(row['input_hex'])
        stream = preimage(row['kind'], parse(raw))
        require(stream.hex() == row['expected_stream_hex'], row['name']+' preimage mismatch')
        require(hashlib.sha256(stream).hexdigest() == row['expected_sha256'], row['name']+' digest mismatch')
    for row in doc['rejected']:
        try:
            value = parse(bytes.fromhex(row['input_hex']))
            if 'schema' in row:
                require(matches(SCHEMAS[row['schema']], value), 'schema rejection')
            else:
                preimage(row['kind'], value)
        except (ValueError, TypeError, UnicodeError, KeyError):
            continue
        raise ValueError('accepted rejected vector '+row['name'])
    by = {r['name']:r for r in doc['vectors']}
    require(by['ascii-empty-body']['expected_sha256'] == by['json-reordered']['expected_sha256'] == by['json-escaped']['expected_sha256'], 'semantic order/escape invariance')
    for a, b in [('lf','crlf'),('nfc','nfd'),('install-absence','install-present-zero')]:
        require(by[a]['expected_sha256'] != by[b]['expected_sha256'], a+' distinction')
    # Confirm that all fields of all layouts bind their semantics. This does not
    # implement operation-key lookup or authorize a caller to submit anything.
    mutations = 0
    for row in doc['vectors']:
        value = parse(bytes.fromhex(row['input_hex']))
        baseline = row['expected_sha256']
        for key, old in value.items():
            altered = dict(value)
            if key.endswith('Id'):
                altered[key] = 'ffffffff-eeee-4ddd-8ccc-bbbbbbbbbbbb'
            elif key.endswith('Digest'):
                altered[key] = 'ab'*32
            elif key == 'formatVersion':
                altered[key] = old+1
            elif key in ('previewVersion','baseRevision','registrationVersion'):
                altered[key] = '2' if old != '2' else '3'
            elif key == 'kind':
                if row['kind'] == 'candidate':
                    altered['kind'] = 'update-text' if old == 'create-text' else 'create-text'
                    if altered['kind'] == 'update-text':
                        altered['baseDigest'] = '00'*32
                    else:
                        altered.pop('baseDigest',None)
                else:
                    altered[key] = 'register' if old != 'register' else 'relink'
            elif key == 'databaseRelativePath':
                continue  # invariant path, rejection is a separate fixed vector
            else:
                altered[key] = old+'x'
            require(hashlib.sha256(preimage(row['kind'], altered)).hexdigest() != baseline, 'unbound field '+key)
            mutations += 1
    # JSON Schema integers denote mathematical integers; do not narrow 2 to an
    # int-only lexical form or route large values through binary floating point.
    for wire in ('2','2.0','2e0','20e-1','0.2e1'):
        inspection = parse(('{"workspaceId":"00112233-4455-4677-8899-aabbccddeeff","tenantId":"00000002-4455-4677-8899-aabbccddeeff","formatVersion":'+wire+',"integrity":"valid","accessMode":"none"}').encode())
        require(matches(SCHEMAS['InspectionView'], inspection), 'mathematical integer '+wire)
    # format is annotation-only in the frozen 2020-12 dialect. Still enforce
    # CandidateView.expiresAt's string type and its required-property status.
    cid = '00112233-4455-4677-8899-aabbccddeeff'
    candidate_view = dict(workspaceId=cid, candidateId=cid, resourceId=cid,
                          kind='create-text', baseRevision='0', title='title', body='',
                          contentDigest='00'*32, candidateDigest='aa'*32,
                          previewVersion='1', validation=dict(state='valid'),
                          diff=dict(afterTitle='title', afterBody=''))
    for expiry in ('2026-10-18T16:00:00Z', '2026-10-18t16:00:00z', '2016-12-31T23:59:60Z', 'not-a-date', ''):
        candidate_view['expiresAt'] = expiry
        require(matches(SCHEMAS['CandidateView'], candidate_view), 'format annotation string rejected')
    for invalid in (None, 123, True, [], {}):
        candidate_view['expiresAt'] = invalid
        require(not matches(SCHEMAS['CandidateView'], candidate_view), 'expiry type enforcement lost')
    del candidate_view['expiresAt']
    require(not matches(SCHEMAS['CandidateView'], candidate_view), 'expiry required enforcement lost')
    return dict(result='passed',vectors=len(doc['vectors']),rejected=len(doc['rejected']),semantic_mutations=mutations,
                annotation_strings=5, annotation_type_or_absence_rejections=6,
                fixture_sha256=hashlib.sha256(TARGET.read_bytes()).hexdigest(),scope='independent bytes/strict fixtures only; no authorization/storage claim')


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--write',action='store_true',help='initial independent oracle freeze; never consumes Go output')
    args = ap.parse_args()
    if args.write:
        require(not TARGET.exists(), 'refuse to overwrite existing frozen expected; review explicitly')
        TARGET.parent.mkdir(parents=True,exist_ok=True)
        TARGET.write_bytes((json.dumps(seeds(),ensure_ascii=True,indent=2)+'\n').encode())
    print(json.dumps(verify(json.loads(TARGET.read_bytes())),ensure_ascii=True))


if __name__ == '__main__':
    try:
        main()
    except (ValueError,KeyError,TypeError,UnicodeError) as e:
        print(f'V002b independent fixture check failed: {e}',file=sys.stderr)
        raise SystemExit(1)
