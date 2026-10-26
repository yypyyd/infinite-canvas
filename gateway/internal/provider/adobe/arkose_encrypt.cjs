// Offline computation only. HTTP and account credentials stay in the Go client.
const vm = require('node:vm');
const { webcrypto } = require('node:crypto');
let input = '';
process.stdin.setEncoding('utf8');
process.stdin.on('data', chunk => {
  input += chunk;
  if (input.length > 3 * 1024 * 1024) process.exit(1);
});
process.stdin.on('end', async () => {
  const watchdog = setTimeout(() => process.exit(1), 10000);
  try {
    const { source, profile, userAgent, publicKey } = JSON.parse(input);
    const marker = ',Pn=n(4166),In=n(2389)';
    if (!source.includes(marker)) throw Error('unsupported SDK');
    // Stop before the UI bootstraps. Only the SDK serialization/encryption is used.
    const isolatedSource = source.replace(marker,
      ';window.__encrypt=xn;window.__hash=le.K;window.__build=O.jt;window.__version=O.i8;return;var Pn=n(4166),In=n(2389)');
    class Element {
      matches() { return false; }
      closest() { return null; }
      getAttribute(key) { return key === 'src' ? this.src : null; }
    }
    const script = Object.assign(new Element(), {
      src: `https://arks-client.adobe.com/v2/${publicKey}/api.js`,
      dataset: { callback: 'setupDetection' },
    });
    const document = {
      currentScript: script,
      getElementsByTagName: name => name === 'script' ? [script] : [],
      querySelector: () => null,
      querySelectorAll: () => [],
      createElement: () => Object.assign(new Element(), { style: {}, setAttribute() {}, appendChild() {} }),
      addEventListener() {}, documentElement: {}, body: {},
    };
    const sandbox = {
      document, Element, location: new URL('https://firefly.adobe.com/generate/image'),
      navigator: { userAgent, language: 'en-US' },
      crypto: webcrypto, TextEncoder, TextDecoder, URL, URLSearchParams, Uint8Array, ArrayBuffer,
      btoa: value => Buffer.from(value, 'binary').toString('base64'),
      atob: value => Buffer.from(value, 'base64').toString('binary'),
      setTimeout, clearTimeout, addEventListener() {}, performance,
      console: { log() {}, warn() {}, error() {} },
    };
    sandbox.window = sandbox;
    sandbox.self = sandbox;
    vm.createContext(sandbox);
    vm.runInContext(isolatedSource, sandbox, { timeout: 5000 });
    const sessionID = webcrypto.randomUUID();
    const fields = profile.find(item => item.key === 'enhanced_fp').value;
    for (const field of fields) {
      if (field.key === '4b4b269e68') field.value = sessionID;
      if (field.key === 'lfasdgs') field.value = sandbox.__build;
    }
    const version = source.match(/u="([a-f0-9]{32})",s="script"/);
    if (version) fields.find(item => item.key === '6a62b2a558').value = version[1];
    profile.find(item => item.key === 'n').value = Buffer.from(String(Math.floor(Date.now() / 1000))).toString('base64');
    const features = profile.find(item => item.key === 'fe').value;
    profile.find(item => item.key === 'f').value = sandbox.__hash(features.join(';'));
    profile.find(item => item.key === 'ife_hash').value = sandbox.__hash(features.join(', '), 38);
    const encrypted = await sandbox.__encrypt(profile, 'https://arks-client.adobe.com', publicKey);
    if (typeof encrypted?.data !== 'string' || encrypted.data.length < 100 || !encrypted.timestamp) throw Error('invalid SDK result');
    // Go reads stdout through a pipe. Exiting before its asynchronous write
    // completes truncates larger results at the pipe capacity (often 8192 bytes).
    process.stdout.write(JSON.stringify({ ...encrypted, sessionID, buildID: sandbox.__build, version: sandbox.__version }), err => {
      clearTimeout(watchdog);
      process.exit(err ? 1 : 0);
    });
  } catch {
    clearTimeout(watchdog);
    process.exit(1);
  }
});
