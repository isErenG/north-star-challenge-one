import { spawn, spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
if (existsSync('.env')) process.loadEnvFile('.env');
const go = existsSync('.context/toolchain/go/bin/go')
  ? resolve('.context/toolchain/go/bin/go')
  : 'go';
if (spawnSync(go, ['version']).status !== 0) {
  console.error('Go is required. Run npm run setup:go first.');
  process.exit(1);
}
const env = { ...process.env, GOTOOLCHAIN: 'local' };
const binary = resolve('.context/kbo-review');
const built = spawnSync(go, ['build', '-o', binary, './cmd/kbo-review'], {
  env,
  stdio: 'inherit'
});
if (built.status !== 0) process.exit(built.status ?? 1);
const api = spawn(binary, [], { env, stdio: 'inherit' });
const web = spawn(
  process.execPath,
  [
    'node_modules/vite/bin/vite.js',
    '--host',
    '127.0.0.1',
    ...process.argv.slice(2)
  ],
  { stdio: 'inherit' }
);
let stopping = false;
function stop(code = 0) {
  if (stopping) return;
  stopping = true;
  api.kill('SIGTERM');
  web.kill('SIGTERM');
  setTimeout(() => process.exit(code), 200);
}
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => stop());
api.on('exit', (code) => stop(code || 0));
web.on('exit', (code) => stop(code || 0));
