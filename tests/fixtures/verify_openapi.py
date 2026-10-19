"""V002a repository-specific structural/behavioral freeze checks (stdlib only).
Not a runtime authorization test or a complete OpenAPI implementation.
"""
from __future__ import annotations
import copy
import hashlib
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[2]
CONTRACT = ROOT / 'contracts/openapi.json'
BEHAVIOR = ROOT / 'specs/001-host-workspace-foundation/contracts/api.md'


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f'duplicate JSON key: {key}')
        result[key] = value
    return result


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate(doc, behavior):
    require(doc.get('openapi') == '3.1.0', 'expected pinned OpenAPI 3.1.0')
    require(doc.get('security') == [{'localSession': []}], 'global bearer requirement changed')
    require(doc['components']['securitySchemes']['localSession']['scheme'] == 'bearer', 'bearer scheme changed')
    expected = {(method.lower(), path) for method, path in re.findall(r'\| (GET|POST) `([^`]+)`', behavior)}
    actual = {(method, path) for path, operations in doc['paths'].items() for method in operations}
    require(len(expected) == 16 and actual == expected, f'operation mismatch: {actual ^ expected}')
    schemas = doc['components']['schemas']
    ids = set()
    checked_refs = set()

    def walk(value):
        if isinstance(value, dict):
            if '$ref' in value:
                target = value['$ref']
                require(target.startswith('#/'), f'external reference: {target}')
                node = doc
                for part in target[2:].split('/'):
                    key = part.replace('~1', '/').replace('~0', '~')
                    require(key in node, f'unresolved reference: {target}')
                    node = node[key]
                checked_refs.add(target)
            if value.get('type') == 'object':
                require(value.get('additionalProperties') is False, 'object permits unknown fields')
                props = value.get('properties', {})
                require(set(value.get('required', [])) <= set(props), 'required field not defined')
            for child in value.values():
                walk(child)
        elif isinstance(value, list):
            for child in value:
                walk(child)
    walk(doc)
    for path, operations in doc['paths'].items():
        for method, operation in operations.items():
            name = operation['operationId']
            require(name not in ids, 'duplicate operationId')
            ids.add(name)
            require('security' not in operation, 'operation overrides bearer requirement')
            require(operation.get('x-authorization-before-target-access') is True, 'missing pre-access authorization')
            require(bool(operation.get('x-required-action')), 'missing action requirement')
            params = operation.get('parameters', [])
            required_path = set(re.findall(r'\{([^}]+)\}', path))
            actual_path = {p['name'] for p in params if p['in'] == 'path' and p.get('required')}
            require(required_path == actual_path, 'path binding mismatch')
            require(len(params) == len({(p['in'], p['name']) for p in params}), 'duplicate parameter')
            if 'requestBody' in operation:
                content = operation['requestBody']['content']
                require(set(content) == {'application/json'}, 'unexpected request media type')
                request = schemas[content['application/json']['schema']['$ref'].split('/')[-1]]
                require(not {'installationId', 'tenantId', 'principalId'} & set(request['properties']), 'client-supplied identity context')
                rows = re.findall(r'\| POST `([^`]+)` \| ([^|]+) \|', behavior)
                core = next(fields.strip() for route, fields in rows if route == path)
                expected_fields = {field.strip().removesuffix('?') for field in core.split(',')}
                require(set(request['properties']) == expected_fields, f'behavior input field mismatch: {name}')
            for status, response in operation['responses'].items():
                require(re.fullmatch(r'[1-5][0-9]{2}', status) is not None, 'invalid status')
                require(set(response['content']) == {'application/json'}, 'unexpected response media type')
                if int(status) >= 400:
                    require(response['content']['application/json']['schema'] == {'$ref':'#/components/schemas/ErrorView'}, 'unsafe error shape')
                if status == '202':
                    require(response['content']['application/json']['schema'] == {'$ref':'#/components/schemas/OperationView'}, 'accepted is not an operation')
                    require('Location' in response.get('headers', {}), 'missing accepted operation locator')
    policy = doc['x-operation-policy']
    require(policy['workspaceKey'] == ['workspaceId', 'operationId'], 'workspace key scope changed')
    require(policy['installationKey'] == ['installationId', 'operationId'], 'installation key scope changed')
    require('before epoch/expiry/base' in policy['existingKey'], 'receipt precedence missing')
    require('missing receipt alone is not rollback' in policy['unknown'], 'unknown boundary missing')
    require('never upgrade or seed' in policy['compatibility'], 'compatibility rejection missing')
    transport = doc['x-transport-policy']
    require(transport['requestMaxBytes'] == 1048576, 'request limit drift')
    require('reject any Origin' in transport['origin'], 'Origin boundary changed')
    require(transport['disconnect'] == 'detach waiter only; never cancel accepted business operation', 'disconnect cancellation drift')
    require(schemas['Body'].get('x-max-utf8-bytes') == 65536, 'UTF-8 byte limit changed')
    require(schemas['Title'].get('maxLength') == 256, 'title scalar limit changed')
    require(schemas['Revision'].get('x-maximum-decimal') == '9223372036854775807', 'revision range drift')
    for dto in ('PrincipalView','WorkspaceView','TextRecordView','CandidateView','OperationView','Receipt','ErrorView'):
        require(not {'path','token','secret','sql','database'} & set(schemas[dto]['properties']), f'ordinary DTO disclosure: {dto}')
    for name in ('PreviewRecord','CandidateView'):
        require(bool(schemas[name].get('allOf')), f'missing create/update distinction: {name}')
    require('Run' in doc['x-deferred-capabilities'] and 'backup' in doc['x-deferred-capabilities'], 'deferred boundary missing')
    return {'operations':len(ids),'schemas':len(schemas),'resolved_reference_targets':len(checked_refs)}


def self_test(doc, behavior):
    mutations = [
        lambda d: d.update(security=[]),
        lambda d: d['components']['schemas']['Body'].update({'$ref':'#/components/schemas/Missing'}),
        lambda d: d['components']['schemas']['CommitRecord'].update(additionalProperties=True),
        lambda d: d['components']['schemas']['CommitRecord']['properties'].update(principalId={'type':'string'}),
        lambda d: d['x-operation-policy'].update(workspaceKey=['operationId']),
        lambda d: d['paths']['/v1/context']['get'].update(security=[]),
        lambda d: d['paths']['/v1/context']['get']['responses']['200']['content']['application/json'].update(schema={'$ref':'https://example.invalid/contract'}),
        lambda d: d['components']['schemas']['Revision'].update({'x-maximum-decimal':'18446744073709551615'}),
    ]
    for number, mutate in enumerate(mutations):
        altered = copy.deepcopy(doc)
        mutate(altered)
        try:
            validate(altered, behavior)
        except (ValueError, KeyError):
            continue
        raise ValueError(f'checker accepted negative mutation {number}')
    try:
        json.loads('{"a":1,"a":2}',object_pairs_hook=unique_object)
    except ValueError:
        return len(mutations)+1
    raise ValueError('duplicate-key checker ineffective')


def main():
    raw = CONTRACT.read_bytes()
    require(not raw.startswith(b'\xef\xbb\xbf'), 'BOM not allowed')
    doc = json.loads(raw.decode('utf-8'),object_pairs_hook=unique_object)
    behavior = BEHAVIOR.read_text(encoding='utf-8')
    result = validate(doc,behavior)
    result.update(negative_checker_tests=self_test(doc,behavior),sha256=hashlib.sha256(raw).hexdigest(),result='passed',scope='V002a static structure only; V002b, authorization and business behavior unverified')
    print(json.dumps(result,ensure_ascii=False))


if __name__ == '__main__':
    try:
        main()
    except (ValueError,KeyError,TypeError) as error:
        print(f'V002a failed: {error}',file=sys.stderr)
        raise SystemExit(1)
