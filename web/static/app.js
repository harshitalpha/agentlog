// agentlog UI entry point. Vanilla ES modules, no build step, no dependencies.

const $ = (selector) => document.querySelector(selector);

async function getJSON(url) {
  const res = await fetch(url, { cache: 'no-store' });
  if (!res.ok) throw new Error(`${url}: HTTP ${res.status}`);
  return res.json();
}

async function boot() {
  try {
    const repo = await getJSON('/api/repo');
    $('#repoName').textContent = repo.name;
    $('#branch').textContent = repo.branch;
    document.title = `agentlog · ${repo.name}`;
  } catch (err) {
    $('#live').classList.add('off');
    $('#live').title = 'disconnected';
    console.error(err);
  }
}

boot();
