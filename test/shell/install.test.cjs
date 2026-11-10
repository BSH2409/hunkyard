const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const crypto = require('node:crypto');
const { spawnSync } = require('node:child_process');

const installer = path.resolve(__dirname, '../../install.sh');
const bash = spawnSync('which', ['bash'], { encoding: 'utf8' }).stdout.trim();

function fixture(t, options = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'hunkyard-install-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  const bin = path.join(root, 'bin');
  const home = path.join(root, 'home with spaces');
  fs.mkdirSync(bin);
  fs.mkdirSync(home);
  for (const name of ['mktemp', 'mkdir', 'chmod', 'mv', 'rm', 'awk', 'grep', 'git', options.shasum ? 'shasum' : 'sha256sum']) {
    if (name === options.missing) continue;
    const found = spawnSync('which', [name], { encoding: 'utf8' }).stdout.trim();
    assert.ok(found, `${name} required to run installer tests`);
    fs.symlinkSync(found, path.join(bin, name));
  }
  const payload = '#!/bin/sh\nprintf "hunkyard v9.8.7\\n"\n';
  const checksum = crypto.createHash('sha256').update(payload).digest('hex');
  fs.writeFileSync(path.join(bin, 'uname'), '#!/bin/sh\ncase "$1" in -s) echo "$TEST_OS";; -m) echo "$TEST_ARCH";; esac\n', { mode: 0o755 });
  if (options.missing !== 'curl') fs.writeFileSync(path.join(bin, 'curl'), `#!${process.execPath}
const fs = require('node:fs');
const args = process.argv.slice(2), url = args.at(-1);
fs.appendFileSync(process.env.TEST_LOG, url + '\\n');
if (url.endsWith('/latest')) { process.stdout.write('https://github.com/BSH2409/hunkyard/releases/tag/v9.8.7'); process.exit(0); }
if (process.env.TEST_FAILURE === 'download') process.exit(22);
let content = ${JSON.stringify(payload)};
if (url.endsWith('/checksums.txt')) {
 const digest = process.env.TEST_FAILURE === 'mismatch' ? '0'.repeat(64) : ${JSON.stringify(checksum)};
 content = process.env.TEST_FAILURE === 'missing-checksum' ? '' : digest + '  hunkyard-' + process.env.TEST_ASSET + '\\n';
}
fs.writeFileSync(args[args.indexOf('--output') + 1], content);
`, { mode: 0o755 });
  const platform = options.os || 'Linux';
  const arch = options.arch || 'aarch64';
  const installDir = path.join(home, '.local', 'bin');
  const env = { ...process.env, PATH: bin, HOME: home, SHELL: options.shell || '/bin/bash', HUNKYARD_VERSION: '', HUNKYARD_INSTALL_DIR: '', TEST_OS: platform, TEST_ARCH: arch, TEST_ASSET: `${platform === 'Darwin' ? 'darwin' : 'linux'}-${arch === 'x86_64' ? 'amd64' : 'arm64'}`, TEST_LOG: path.join(root, 'downloads'), TEST_FAILURE: options.failure || '' };
  return { root, home, installDir, env, run(overrides = {}) { return spawnSync(bash, [installer], { env: { ...env, ...overrides }, encoding: 'utf8' }); } };
}

for (const [platform, arch, shasum, shell, profile] of [
  ['Linux', 'aarch64', false, '/bin/bash', '.bashrc'],
  ['Linux', 'x86_64', false, '/bin/bash', '.bashrc'],
  ['Darwin', 'arm64', true, '/bin/zsh', '.zshrc'],
  ['Darwin', 'x86_64', true, '/bin/bash', '.bash_profile'],
]) {
  test(`installs ${platform}/${arch} and configures PATH once`, t => {
    const f = fixture(t, { os: platform, arch, shasum, shell });
    const first = f.run();
    assert.equal(first.status, 0, first.stderr);
    assert.match(fs.readFileSync(path.join(f.root, 'downloads'), 'utf8'), new RegExp(`/v9.8.7/hunkyard-${f.env.TEST_ASSET}`));
    assert.ok(fs.statSync(path.join(f.installDir, 'hunkyard')).mode & 0o111);
    const profilePath = path.join(f.home, profile);
    const startup = fs.readFileSync(profilePath, 'utf8');
    const configured = spawnSync(bash, ['-c', 'source "$1"; command -v hunkyard', 'test', profilePath], { env: f.env, encoding: 'utf8' });
    assert.equal(configured.stdout.trim(), path.join(f.installDir, 'hunkyard'));
    const second = f.run();
    assert.equal(second.status, 0, second.stderr);
    assert.equal(fs.readFileSync(profilePath, 'utf8'), startup);
    assert.deepEqual(fs.readdirSync(f.installDir), ['hunkyard']);
  });
}

for (const failure of ['download', 'mismatch', 'missing-checksum']) {
  test(`${failure} preserves the installed binary and cleans staging files`, t => {
    const f = fixture(t, { failure });
    fs.mkdirSync(f.installDir, { recursive: true });
    fs.writeFileSync(path.join(f.installDir, 'hunkyard'), 'previous binary');
    const result = f.run();
    assert.notEqual(result.status, 0);
    assert.equal(fs.readFileSync(path.join(f.installDir, 'hunkyard'), 'utf8'), 'previous binary');
    assert.deepEqual(fs.readdirSync(f.installDir), ['hunkyard']);
    assert.equal(fs.existsSync(path.join(f.home, '.bashrc')), false);
  });
}

for (const [options, overrides, message] of [
  [{ missing: 'git' }, {}, /git is required/],
  [{ missing: 'curl' }, {}, /curl is required/],
  [{ missing: 'sha256sum' }, {}, /SHA-256 verification requires/],
  [{ os: 'FreeBSD' }, {}, /supports Linux and macOS/],
  [{ arch: 'riscv64' }, {}, /Unsupported CPU architecture/],
  [{}, { HUNKYARD_VERSION: '../../bad' }, /Invalid release version/],
  [{}, { HUNKYARD_INSTALL_DIR: 'relative' }, /must be an absolute path/],
]) {
  test(`rejects ${message}`, t => {
    const f = fixture(t, options);
    const result = f.run(overrides);
    assert.notEqual(result.status, 0);
    assert.match(result.stderr, message);
    assert.equal(fs.existsSync(f.installDir), false);
  });
}

test('supports a pinned release and an install directory already on PATH', t => {
  const f = fixture(t);
  const result = f.run({ HUNKYARD_VERSION: 'v1.0.4', HUNKYARD_INSTALL_DIR: f.installDir, PATH: `${f.installDir}:${f.env.PATH}` });
  assert.equal(result.status, 0, result.stderr);
  const downloads = fs.readFileSync(path.join(f.root, 'downloads'), 'utf8');
  assert.doesNotMatch(downloads, /\/latest/);
  assert.match(downloads, /\/v1\.0\.4\/hunkyard-linux-arm64/);
  assert.equal(fs.existsSync(path.join(f.home, '.bashrc')), false);
});


test('preserves an existing macOS login profile and its startup behavior', t => {
  const f = fixture(t, { os: 'Darwin', arch: 'arm64' });
  const profile = path.join(f.home, '.profile');
  fs.writeFileSync(profile, 'export EXISTING_SETTING=keep\n');
  const result = f.run();
  assert.equal(result.status, 0, result.stderr);
  assert.match(fs.readFileSync(profile, 'utf8'), /^export EXISTING_SETTING=keep\n/);
  assert.equal(fs.existsSync(path.join(f.home, '.bash_profile')), false);
});

test('an incomplete piped script cannot begin installation', t => {
  const f = fixture(t);
  const script = fs.readFileSync(installer, 'utf8');
  const truncated = script.slice(0, script.indexOf('  mv -f'));
  const result = spawnSync(bash, [], { input: truncated, env: f.env, encoding: 'utf8' });
  assert.notEqual(result.status, 0);
  assert.equal(fs.existsSync(f.installDir), false);
});

test('refuses to replace a directory at the binary destination', t => {
  const f = fixture(t);
  const target = path.join(f.installDir, 'hunkyard');
  fs.mkdirSync(target, { recursive: true });
  fs.writeFileSync(path.join(target, 'keep.txt'), 'keep');
  const result = f.run();
  assert.notEqual(result.status, 0);
  assert.match(result.stderr, /is a directory/);
  assert.equal(fs.readFileSync(path.join(target, 'keep.txt'), 'utf8'), 'keep');
});
