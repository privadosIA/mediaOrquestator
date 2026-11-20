// scripts/hyperframes-runtime-build.cjs: bundles the runtime of HyperFrames (packages/core/src/runtime/entry.ts of a clone) into one
// script, the way its own build does (esbuild, iife, es2020, minified), resolving @hyperframes/parsers/* through the `bun` entries of
// the exports of the sibling package, which is how its workspace reads them. Called by get-hyperframes-runtime.sh with:
//   HF_CLONE (the clone), ESBUILD_JS (the esbuild package), ESBUILD_BIN (its binary), NM (node_modules with the dependencies), OUT
const path = require('path'), fs = require('fs');
process.env.ESBUILD_BINARY_PATH = process.env.ESBUILD_BIN;
const esbuild = require(process.env.ESBUILD_JS);
const core = path.join(process.env.HF_CLONE, 'packages/core'), parsers = path.join(process.env.HF_CLONE, 'packages/parsers');
const pk = JSON.parse(fs.readFileSync(path.join(parsers, 'package.json'), 'utf8'));
const plugin = {
  name: 'hyperframes-workspace',
  setup(b) {
    b.onResolve({ filter: /^@hyperframes\/parsers(\/.*)?$/ }, (a) => {
      const sub = a.path === '@hyperframes/parsers' ? '.' : './' + a.path.slice('@hyperframes/parsers/'.length);
      const e = pk.exports[sub];
      if (!e) return { errors: [{ text: 'the parsers package has no export ' + sub }] };
      return { path: path.resolve(parsers, typeof e === 'string' ? e : (e.bun || e.import)) };
    });
  },
};
esbuild.build({
  entryPoints: [path.join(core, 'src/runtime/entry.ts')], bundle: true, platform: 'browser', format: 'iife', target: ['es2020'],
  minify: true, legalComments: 'none', define: { console: 'globalThis.console' }, nodePaths: [process.env.NM], plugins: [plugin],
  outfile: process.env.OUT, logLevel: 'error',
}).then(() => console.log('built ' + fs.statSync(process.env.OUT).size + ' bytes')).catch(() => process.exit(1));
