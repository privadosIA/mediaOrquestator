// scripts/extract-shader-transitions.cjs: writes the fragment shaders of @hyperframes/shader-transitions (Apache-2.0) to a JSON file, so the
// server can draw them without Node. A developer step, run when the pinned commit of HyperFrames changes (the result is committed):
//   HF_CLONE=<clone> ESBUILD_JS=<esbuild package> ESBUILD_BIN=<its binary> OUT=internal/video/shadertrans/shaders.json node scripts/extract-shader-transitions.cjs
const path = require('path'), fs = require('fs');
process.env.ESBUILD_BINARY_PATH = process.env.ESBUILD_BIN;
const esbuild = require(process.env.ESBUILD_JS);
const src = path.join(process.env.HF_CLONE, 'packages/shader-transitions/src/shaders');
const out = esbuild.buildSync({ entryPoints: [path.join(src, 'registry.ts')], bundle: true, format: 'cjs', platform: 'node', write: false, logLevel: 'error' });
const mod = { exports: {} };
new Function('module', 'exports', 'require', out.outputFiles[0].text)(mod, mod.exports, require);
const common = esbuild.buildSync({ entryPoints: [path.join(src, 'common.ts')], bundle: true, format: 'cjs', platform: 'node', write: false, logLevel: 'error' });
const cm = { exports: {} };
new Function('module', 'exports', 'require', common.outputFiles[0].text)(cm, cm.exports, require);
const result = { vertex: cm.exports.vertSrc, shaders: {} };
for (const name of mod.exports.SHADER_NAMES) result.shaders[name] = mod.exports.getFragSource(name);
fs.writeFileSync(process.env.OUT, JSON.stringify(result, null, 1) + '\n');
console.log('wrote ' + Object.keys(result.shaders).length + ' shaders');
