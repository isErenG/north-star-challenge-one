import { spawnSync } from 'node:child_process';
import { existsSync } from 'node:fs';
import { resolve } from 'node:path';
const go = existsSync('.context/toolchain/go/bin/go')
  ? resolve('.context/toolchain/go/bin/go')
  : 'go';
process.exit(
  spawnSync(go, ['test', '-race', './...'], {
    cwd: 'server',
    stdio: 'inherit',
    env: { ...process.env, GOTOOLCHAIN: 'local' }
  }).status ?? 1
);
