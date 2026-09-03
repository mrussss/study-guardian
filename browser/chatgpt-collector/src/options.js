const input = document.querySelector('#token');
const status = document.querySelector('#status');
const current = await chrome.storage.local.get({ collector_token: '' });
input.value = current.collector_token;
document.querySelector('#save').addEventListener('click', async () => {
  await chrome.storage.local.set({ collector_token: input.value.trim() });
  status.textContent = ' saved';
});
