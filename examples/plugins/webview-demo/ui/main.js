document.querySelector('#button').addEventListener('click', () => {
  document.querySelector('#status').textContent = '交互正常：插件自己的 JavaScript 已执行。';
});

document.querySelector('#read-settings').addEventListener('click', async () => {
  const status = document.querySelector('#settings-status');
  try {
    const response = await fetch('/_api/settings/message');
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const data = await response.json();
    status.textContent = `设置值：${data.value}`;
  } catch (error) {
    status.textContent = `读取失败：${error.message}`;
  }
});
