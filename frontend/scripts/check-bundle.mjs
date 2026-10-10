import {readFile, readdir, stat} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';

const scriptDirectory = path.dirname(fileURLToPath(import.meta.url));
const webDirectory = path.resolve(scriptDirectory, '../../internal/httpapi/web');
const indexHtml = await readFile(path.join(webDirectory, 'index.html'), 'utf8');

const initialScript = indexHtml.match(/<script[^>]+src="([^"]+\.js)"/)?.[1];
const initialStyle = indexHtml.match(/<link[^>]+href="([^"]+\.css)"/)?.[1];

if (!initialScript || !initialStyle) {
  throw new Error('Could not identify the initial JavaScript and CSS assets in index.html.');
}

function assetPath(publicPath) {
  return path.join(webDirectory, publicPath.replace(/^\/app\//, ''));
}

async function enforceBudget(label, file, limitKiB) {
  const bytes = (await stat(file)).size;
  const limit = limitKiB * 1024;
  if (bytes > limit) {
    throw new Error(`${label} is ${(bytes / 1024).toFixed(1)} KiB; budget is ${limitKiB} KiB.`);
  }
  console.log(`✓ ${label}: ${(bytes / 1024).toFixed(1)} KiB / ${limitKiB} KiB`);
}

await enforceBudget('Initial JavaScript', assetPath(initialScript), 250);
await enforceBudget('Initial CSS', assetPath(initialStyle), 16);

const assets = await readdir(path.join(webDirectory, 'assets'));
const builderScript = assets.find((name) => /^Builder-.*\.js$/.test(name));
if (!builderScript) {
  throw new Error('The DAG Builder must remain in a separately loaded JavaScript chunk.');
}
await enforceBudget(
  'Lazy DAG Builder JavaScript',
  path.join(webDirectory, 'assets', builderScript),
  200,
);
