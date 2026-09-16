import { mkdir, writeFile } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { execFileSync } from 'node:child_process';
const os = process.platform === 'darwin' ? 'darwin' : 'linux';
const arch = process.arch === 'arm64' ? 'arm64' : 'amd64';
const releases = await (await fetch('https://go.dev/dl/?mode=json')).json();
const file = releases[0].files.find(
  (f) => f.os === os && f.arch === arch && f.kind === 'archive'
);
if (!file) throw new Error('No Go archive for this platform');
console.log(`Installing ${file.filename} into .context (workspace only)`);
const bytes = Buffer.from(
  await (await fetch(`https://go.dev/dl/${file.filename}`)).arrayBuffer()
);
if (createHash('sha256').update(bytes).digest('hex') !== file.sha256)
  throw new Error('Go checksum mismatch');
await mkdir('.context/toolchain', { recursive: true });
await writeFile('.context/toolchain/go.tar.gz', bytes);
execFileSync('tar', [
  '-xzf',
  '.context/toolchain/go.tar.gz',
  '-C',
  '.context/toolchain'
]);
