# StudyGuardian ChatGPT Collector

Manifest V3 baseline collector. It only reads the current ChatGPT DOM and
starts with an in-memory baseline, so messages already visible when the
content script attaches are not imported automatically. The background worker
is the only component that reads the scoped collector token and performs
localhost HTTP requests. The queue is bounded to 1000 items / 10 MiB and
prefers dropping finalized assistant payloads before user prompts.

Install dependencies with `npm install` in this directory, run `npm test`,
then load the directory as an unpacked extension in Chrome. Configure the
token from the extension options page; never put the Supervisor main token in
the extension.
