# StudyGuardian ChatGPT Collector

Manifest V3 baseline collector. It only reads the current ChatGPT DOM and
starts with an in-memory baseline, so messages already visible when the
content script attaches are not imported automatically. The background worker
is the only component that reads the scoped collector token and performs
localhost HTTP requests. The queue is bounded to 1000 items / 10 MiB and
prefers dropping finalized assistant payloads before user prompts.

Frozen Turn Context is backed by `chrome.storage.session`, so it survives a
Manifest V3 Service Worker restart but is cleared when the browser exits. The
current page's historical baseline remains in Content Script memory.

The readable Content Script modules under `src/` are bundled by
`scripts/build-windows.sh` into `dist/content.js`. The manifest loads this
classic bundle, while the background Service Worker remains an ES module.
The Windows artifact therefore does not require Node or npm on the machine
where Chrome loads the unpacked extension.

The Background Service Worker serializes the complete Collector delivery
pipeline globally, so streaming snapshots cannot overtake one another.

Development source workflow:

```text
npm install
npm test
python3 ../../scripts/bundle-content.py src/content.js dist/content.js
```

After bundling, the current directory can be loaded as an unpacked extension
in Chrome. The recommended Windows workflow is `scripts/build-windows.sh`,
which creates the bundle and complete artifact; load
`D:\StudyGuardianDev\browser\chatgpt-collector` after deployment. A source
clone that has not run the bundle step is not loadable because `dist/` is a
generated, ignored directory.

Configure the token from the extension options page; never put the Supervisor
main token in the extension.
